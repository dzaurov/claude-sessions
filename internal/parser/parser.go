// Package parser streams a Claude Code session .jsonl file and extracts
// metadata needed for the ccs index: custom title, first real user message,
// cwd, git branch, message count, and last activity timestamp.
//
// "Real" user message = type:"user" with isMeta=false, isSidechain=false,
// non-empty content that isn't wrapped in <command-name>, <local-command-*>,
// or <system-reminder> tags.
package parser

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

const (
	maxLineSize         = 16 * 1024 * 1024 // 16 MiB
	titleMaxLength      = 200
	signatureProbeLines = 5 // inspect this many lines for session signature
)

type Result struct {
	CustomTitle   string
	FirstUserMsg  string
	Cwd           string
	GitBranch     string
	LastTimestamp string
	MsgCount      int
}

type rawLine struct {
	Type        string          `json:"type"`
	CustomTitle string          `json:"customTitle"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	UserType    string          `json:"userType"`
	Cwd         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Timestamp   string          `json:"timestamp"`
	Message     json.RawMessage `json:"message"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func ParseFile(path string) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	return parse(f)
}

// IsSessionFile returns true if the .jsonl at path looks like a Claude Code
// session log: at least one of the first signatureProbeLines is valid JSON
// containing either a sessionId field, or a top-level "type" field with a
// known value (custom-title, agent-name, user, assistant, system,
// last-prompt, agent-prompt, file-history-snapshot, permission-mode,
// attachment).
//
// This filters out arbitrary .jsonl files (datasets, app logs, etc.) when
// walking the whole disk for sessions.
func IsSessionFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), maxLineSize)
	known := map[string]bool{
		"custom-title":          true,
		"agent-name":            true,
		"user":                  true,
		"assistant":             true,
		"system":                true,
		"last-prompt":           true,
		"agent-prompt":          true,
		"file-history-snapshot": true,
		"permission-mode":       true,
		"attachment":            true,
	}
	for i := 0; i < signatureProbeLines && sc.Scan(); i++ {
		var probe struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(sc.Bytes(), &probe); err != nil {
			continue
		}
		if probe.SessionID != "" {
			return true
		}
		if known[probe.Type] {
			return true
		}
	}
	return false
}

func parse(r io.Reader) (Result, error) {
	var res Result
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxLineSize)
	for sc.Scan() {
		var rl rawLine
		if err := json.Unmarshal(sc.Bytes(), &rl); err != nil {
			continue
		}
		if rl.Timestamp != "" {
			res.LastTimestamp = rl.Timestamp
		}
		switch rl.Type {
		case "custom-title":
			if rl.CustomTitle != "" {
				res.CustomTitle = rl.CustomTitle
			}
		case "user":
			res.MsgCount++
			if rl.IsMeta || rl.IsSidechain {
				continue
			}
			if res.FirstUserMsg != "" {
				continue
			}
			text := extractText(rl.Message)
			if text == "" || isWrapperText(text) {
				continue
			}
			if len(text) > titleMaxLength {
				text = text[:titleMaxLength]
			}
			res.FirstUserMsg = text
			if rl.Cwd != "" {
				res.Cwd = rl.Cwd
			}
			if rl.GitBranch != "" {
				res.GitBranch = rl.GitBranch
			}
		case "assistant":
			res.MsgCount++
		}
		if res.Cwd == "" && rl.Cwd != "" {
			res.Cwd = rl.Cwd
		}
		if res.GitBranch == "" && rl.GitBranch != "" {
			res.GitBranch = rl.GitBranch
		}
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	return res, nil
}

func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m rawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if len(m.Content) == 0 {
		return ""
	}
	if m.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			return s
		}
		return ""
	}
	if m.Content[0] == '[' {
		var blocks []contentBlock
		if err := json.Unmarshal(m.Content, &blocks); err != nil {
			return ""
		}
		parts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func isWrapperText(s string) bool {
	t := strings.TrimSpace(s)
	prefixes := []string{
		"<command-name>",
		"<command-message>",
		"<local-command-stdout>",
		"<local-command-stderr>",
		"<local-command-caveat>",
		"<system-reminder>",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}
