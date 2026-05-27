<h1 align="center">ccs — Claude Code Sessions</h1>

<p align="center">
  <em>Browse and resume every Claude Code chat across every project, from any terminal.</em>
</p>

<p align="center">
  <a href="https://github.com/dzaurov/claude-sessions/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/dzaurov/claude-sessions/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/dzaurov/claude-sessions/releases"><img alt="Release" src="https://img.shields.io/github/v/release/dzaurov/claude-sessions?include_prereleases&sort=semver"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-yellow.svg"></a>
  <a href="https://golang.org/dl/"><img alt="Go" src="https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go"></a>
  <a href="https://github.com/dzaurov/claude-sessions/stargazers"><img alt="Stars" src="https://img.shields.io/github/stars/dzaurov/claude-sessions?style=social"></a>
</p>

---

## The problem

[Claude Code](https://www.anthropic.com/claude-code) stores every chat as
`~/.claude/projects/<encoded-cwd>/<uuid>.jsonl` — keyed by the **working
directory** the session was started in. Sessions are identified only by
UUID; there is no title, no summary, no preview metadata.

The built-in `claude --resume` is hard-bound to your **current** directory.
So to resume a chat you have to:

1. Remember which directory the session was originally started in
2. `cd` there
3. Pick the right UUID out of a list with no preview of what's inside

If you don't remember the directory, the chat is effectively lost. And if a
session was started somewhere unusual — a Docker container mount, a
worktree, an SSH'd dev box, a one-off `/tmp/foo` experiment — `claude
--resume` from your normal terminal will never find it at all.

`ccs` fixes all of this. One ~5 MB Go binary that:

- Lists **every** session from **every** project in a single searchable
  view, regardless of where your terminal is right now.
- Shows the first real user message of each chat as its title — no more
  picking UUIDs blind.
- Optionally sweeps the **whole disk** for session files that live outside
  Claude Code's default `~/.claude/projects/` (Docker, worktrees,
  backups, mounted volumes).
- Resumes the chosen chat **in your current terminal** — `chdir` into the
  session's original cwd and `exec` into `claude --resume <uuid>`.

## What you get

```
┌─ cc-sessions ───────────────────────────────────────────────────┐
│ /jenkins_                                          42 sessions  │
├─────────────────────────────────────────────────────────────────┤
│ [★] 2026-05-15 14:23  auth-service     fix JWT refresh race    │
│     2026-05-12 09:01  notes-cli        rewrite tag storage      │
│ [★] 2026-05-10 18:45  api-gateway      CORS fix /api/track     │
│     2026-05-09 11:30  webapp           refactor websocket retry │
│     ...                                                          │
├─────────────────────────────────────────────────────────────────┤
│ ↑↓ nav · ⏎ resume · / search · p pin · h hide · r rescan · ?   │
└─────────────────────────────────────────────────────────────────┘
```

- **One list, every session.** Indexes `~/.claude/projects/` plus any
  other location where Claude Code session files live (Docker containers,
  custom mounts, backups). Optional `--full-scan` sweeps the whole disk.
- **Human-readable titles.** First real user message of each chat —
  command wrappers (`<command-name>`, `<system-reminder>`, etc.) and meta
  messages are skipped automatically.
- **Fuzzy search** across title, project path, and tags.
- **Pin** important chats to the top, **hide** experimental noise.
- **Resume in the same window.** Hit `Enter` — `ccs` calls
  `os.Chdir` + `syscall.Exec` so `claude --resume <uuid>` takes over the
  current terminal. No new windows, no context switching.
- **Fast.** Parallel parser, mtime-based cache. ~500ms cold,
  ~5ms warm against ~150 MB of session data on a M1.

## Install

### From source

```bash
git clone https://github.com/dzaurov/claude-sessions.git
cd claude-sessions
make install
```

This builds `bin/ccs` and symlinks it into `~/.local/bin/ccs`. Make sure
`~/.local/bin` is in your `$PATH`:

```bash
# add to ~/.zshrc or ~/.bashrc if not already
export PATH="$HOME/.local/bin:$PATH"
```

### Requirements

- Go 1.22+ (only at build time)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed
  and reachable as `claude` on your `$PATH`
- macOS or Linux

## Usage

Run `ccs` from any terminal:

```bash
$ ccs
```

### Hotkeys

| Key | Action |
|-----|--------|
| `↑` / `↓` (or `k`/`j`) | Navigate |
| `Enter` | Resume the selected session in this terminal |
| `Ctrl-F` | **Fork** the session (`claude --resume <uuid> --fork-session`) |
| `F2` | **Rename** session (saves a custom title; the original JSONL is never touched) |
| `/` | Enter fuzzy search |
| `Esc` | Clear search / cancel rename |
| `p` | Pin / unpin |
| `h` | Hide / unhide (does not delete the file) |
| `t` | Toggle visibility of hidden sessions |
| `r` | Force rescan |
| `?` | Show full help |
| `q` / `Ctrl-C` | Quit |

Hotkeys are remappable via `[keys]` in `config.toml` (see below).

By default, sessions are sorted **pinned first**, then by most recent
activity.

### Flags

```
ccs                            # open the TUI
ccs <file.jsonl>               # print metadata for one session
ccs <file.jsonl> --resume      # resume that single session immediately
ccs <file.jsonl> --resume --fork  # fork-resume a single session
ccs --full-scan                # synchronous full-disk discovery scan
ccs --list-json                # dump the current index as JSON
ccs --show-id                  # TUI picker, print UUID to stdout instead of resuming
ccs --show-path                # TUI picker, print file path to stdout
```

A background full-scan automatically runs once every 24 hours so that
sessions Claude Code creates in unusual places (e.g. inside Docker
containers) end up in the index without you doing anything.

`--show-id` and `--show-path` make ccs composable with shell pipelines:

```bash
# Pick a session in the TUI, then resume it with a custom flag combination
claude --resume "$(ccs --show-id)" --model opus

# Pipe the selected JSONL path to another tool (jq, less, your own script)
less "$(ccs --show-path)"
```

If you use the [`CLAUDE_CONFIG_DIR`](https://docs.anthropic.com/en/docs/claude-code/settings)
environment variable to relocate Claude Code's data, ccs respects it
automatically.

## Configuration

`~/.claude/cc-sessions/config.toml` (auto-created with sensible defaults
on first run; uses `$CLAUDE_CONFIG_DIR` if set):

```toml
# Where to look for sessions. Default is Claude Code's canonical location.
roots = ["~/.claude/projects"]

# Arguments appended to every `claude --resume <uuid>` invocation. Replaces
# the older permission_mode option (still honored if present).
default_args = ["--dangerously-skip-permissions"]
# Examples:
#   default_args = ["--dangerously-skip-permissions", "--model", "opus"]
#   default_args = ["--permission-mode", "acceptEdits"]

# Filtering and display.
show_hidden       = false      # hidden sessions visible by default?
show_empty        = false      # show sessions with no real user message
max_title_length  = 80
date_format       = "2006-01-02 15:04"

# Full-disk discovery: walked when --full-scan is invoked, or
# automatically every full_scan_interval_hours in the background.
full_scan_paths           = ["~"]
full_scan_interval_hours  = 24
full_scan_ignore = [
  ".git", "node_modules", "Library", "Pods", "vendor", "target",
  "dist", "build", "__pycache__", ".venv", "venv",
  ".cargo", ".rustup", ".m2", ".gradle", ".docker", ".Trash",
  "testdata",
  # add your own
]

# Optional per-action key overrides. Empty/missing = use the default.
# Values use bubbletea key syntax: "enter", "esc", "ctrl+f", "f2", "/", "k", ...
[keys]
# fork   = "ctrl+f"
# rename = "f2"
# pin    = "p"
# hide   = "h"
```

## How it works

```
~/.claude/projects/<encoded-path>/<uuid>.jsonl   ← source of truth
                       │
                       ▼
            ┌─────────────────────┐
            │  scanner (parallel) │   walks roots, parses JSONL with
            └─────────────────────┘   NumCPU workers
                       │
                       ▼
            ┌─────────────────────┐
            │   cache: index.json │   mtime-keyed, atomic writes
            └─────────────────────┘
                       │
                       ▼
            ┌─────────────────────┐
            │ TUI (bubbletea)     │   list, search, pin/hide
            └─────────────────────┘
                       │ Enter
                       ▼
            ┌─────────────────────┐
            │ launcher: chdir +   │   syscall.Exec("claude", ["claude",
            │ syscall.Exec        │     "--resume", uuid, ...])
            └─────────────────────┘
```

State and config live under `~/.claude/cc-sessions/`:

| File | Purpose |
|------|---------|
| `config.toml` | Your settings |
| `index.json` | Cached session metadata (mtime-invalidated) |
| `meta.json` | Your pins / hides / tags (user state) |
| `state.json` | Bookkeeping (last full-scan timestamp) |

`ccs` is **strictly read-only** against `~/.claude/projects/`. It never
modifies, moves, or deletes anything Claude Code owns.

## Privacy

ccs runs entirely locally. There is no telemetry, no network access, no
upload of any session content anywhere. The cache and metadata live in
`~/.claude/cc-sessions/` and stay there.

## Building & testing

```bash
make test        # unit + integration (47+ tests)
make lint        # go vet + gofmt
make build       # → bin/ccs
make install     # → ~/.local/bin/ccs
make clean
```

## Design docs

For the curious: the original spec and implementation plan are in
[`docs/design.md`](docs/design.md) and
[`docs/implementation-plan.md`](docs/implementation-plan.md).

## Contributing

Bug reports, fixes, and small features are welcome. See
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).

## Related

- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) — the
  CLI ccs is built around
- [raine/claude-history](https://github.com/raine/claude-history) — a
  sibling Rust project with a similar goal. It focuses on **viewing**
  conversations inside the TUI (full transcript rendering, markdown,
  vim-style scrolling, semantic search). ccs focuses on **finding and
  resuming** with full-disk discovery for sessions in unusual locations
  (Docker mounts, worktrees, etc.). Pick whichever fits your workflow —
  or both.
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — the TUI
  framework powering the UI
