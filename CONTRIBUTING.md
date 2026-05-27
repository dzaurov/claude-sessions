# Contributing to ccs

Thanks for taking a look. ccs is a small focused tool; the maintainer is
happy to review well-scoped patches.

## Quick start

```bash
git clone https://github.com/dzaurov/claude-sessions.git
cd claude-sessions
make test       # unit + integration
make lint       # go vet + gofmt
make build      # → bin/ccs
make install    # → ~/.local/bin/ccs
```

You'll need Go 1.22+ and a real `~/.claude/projects/` directory (i.e. a
working Claude Code installation) for manual end-to-end testing.

## What kind of contributions are welcome

- **Bug reports.** Open an issue with: OS, Go version, `ccs --list-json`
  output (redacted), what you expected, what happened.
- **Bug fixes** with a regression test.
- **New features that fit the scope:** anything that helps a developer
  find, organize, or jump back into a Claude Code session faster.
- **Documentation improvements** — examples, edge cases, OS-specific
  caveats.

## What's out of scope

- Modifying or writing to `~/.claude/projects/` content. ccs is strictly
  read-only against Claude Code's own data — that file tree belongs to
  Claude Code.
- Generic chat history tools for other AIs. Other backends are welcome as
  separate sibling projects, but ccs stays focused on Claude Code's JSONL
  format.
- Heavy GUI work. ccs is a TUI by design.

## Code style

- Run `gofmt -w .` before committing.
- Keep packages small and single-purpose (see `internal/` for the pattern).
- New behavior needs a test. Look at `internal/*/_test.go` and `testdata/`
  for examples of the fixtures-based approach used here.
- Comments only when the *why* is non-obvious. Code should explain *what*
  via naming.

## Commits and PRs

- One logical change per commit. Conventional-commit prefixes are nice
  (`feat:`, `fix:`, `docs:`, `test:`, `chore:`) but not required.
- Reference the issue you're addressing in the PR description.
- Don't worry about squashing — the maintainer will rebase as needed.

## Privacy & security

Session data is private by definition. When you submit bug reports or test
fixtures:

- Strip real user prompts, paths, and IPs.
- See `testdata/normal.jsonl` for the kind of synthetic example that's
  acceptable in tests.
- Never commit any `index.json` / `meta.json` content from your own
  `~/.claude/cc-sessions/`.

## License

By submitting a contribution you agree to license it under the project's
[MIT License](LICENSE).
