// Package paths decodes the encoded project folder names that Claude Code
// uses under ~/.claude/projects/. The encoding is lossy (real "-" in paths
// become indistinguishable from separators), so Decode is best-effort and
// callers should prefer the cwd field from the JSONL itself when available.
package paths

import "strings"

func Decode(encoded string) string {
	if encoded == "" {
		return ""
	}
	if strings.HasPrefix(encoded, "-") {
		return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
	}
	return strings.ReplaceAll(encoded, "-", "/")
}

func Encode(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}
