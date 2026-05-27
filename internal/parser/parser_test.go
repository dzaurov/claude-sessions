package parser

import (
	"strings"
	"testing"
)

func TestParseNormal(t *testing.T) {
	r, err := ParseFile("../../testdata/normal.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.CustomTitle != "Cascade merge investigation" {
		t.Errorf("CustomTitle=%q", r.CustomTitle)
	}
	if !strings.HasPrefix(r.FirstUserMsg, "How does cascade merge") {
		t.Errorf("FirstUserMsg=%q", r.FirstUserMsg)
	}
	if r.Cwd != "/Users/alice/Documents/example-project" {
		t.Errorf("Cwd=%q", r.Cwd)
	}
	if r.GitBranch != "main" {
		t.Errorf("GitBranch=%q", r.GitBranch)
	}
	if r.MsgCount < 4 {
		t.Errorf("MsgCount=%d, want >=4", r.MsgCount)
	}
	if r.LastTimestamp == "" {
		t.Errorf("LastTimestamp empty")
	}
}

func TestParseEmpty(t *testing.T) {
	r, err := ParseFile("../../testdata/empty.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.FirstUserMsg != "" || r.CustomTitle != "" || r.MsgCount != 0 {
		t.Errorf("expected zero values, got %+v", r)
	}
}

func TestParseNoUser(t *testing.T) {
	r, err := ParseFile("../../testdata/no_user.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.CustomTitle != "only-system" {
		t.Errorf("CustomTitle=%q", r.CustomTitle)
	}
	if r.FirstUserMsg != "" {
		t.Errorf("FirstUserMsg=%q, want empty", r.FirstUserMsg)
	}
}

func TestParseSystemOnly_skipsMetaSidechainWrappers(t *testing.T) {
	r, err := ParseFile("../../testdata/system_only.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.FirstUserMsg != "" {
		t.Errorf("FirstUserMsg=%q, want empty (all should be skipped)", r.FirstUserMsg)
	}
}

func TestParsePartialCorrupt(t *testing.T) {
	r, err := ParseFile("../../testdata/partial_corrupt.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.FirstUserMsg != "this should be the title" {
		t.Errorf("FirstUserMsg=%q", r.FirstUserMsg)
	}
}

func TestIsSessionFile_acceptsRealSession(t *testing.T) {
	if !IsSessionFile("../../testdata/normal.jsonl") {
		t.Error("expected normal.jsonl to be recognized as session")
	}
	if !IsSessionFile("../../testdata/no_user.jsonl") {
		t.Error("expected no_user.jsonl (has custom-title) to be session")
	}
}

func TestIsSessionFile_rejectsNonSession(t *testing.T) {
	if IsSessionFile("../../testdata/not_a_session.jsonl") {
		t.Error("expected not_a_session.jsonl to be rejected")
	}
}

func TestIsSessionFile_rejectsEmpty(t *testing.T) {
	if IsSessionFile("../../testdata/empty.jsonl") {
		t.Error("expected empty.jsonl to be rejected")
	}
}

func TestParseWrappedContent(t *testing.T) {
	r, err := ParseFile("../../testdata/wrapped_content.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.FirstUserMsg != "Hello from array content" {
		t.Errorf("FirstUserMsg=%q", r.FirstUserMsg)
	}
}
