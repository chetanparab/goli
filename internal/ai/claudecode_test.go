package ai

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chetanparab/goli/internal/links"
)

// fakeClaude writes a stand-in for the claude CLI that prints body.
func fakeClaude(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\n" +
		`if [ -n "$ANTHROPIC_BASE_URL" ]; then echo '{"is_error":true,"result":"env leaked"}'; exit 1; fi` + "\n" +
		"cat >/dev/null\ncat <<'JSON'\n" + body + "\nJSON\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

var catalogLinks = []*links.Link{
	{Key: "cf", URL: "https://dash.cloudflare.com", Desc: "Cloudflare dashboard"},
	{Key: "notes", URL: "https://notes.example.com"},
}

func TestClaudeCodeRank(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "http://should-not-leak")
	bin := fakeClaude(t, `{"is_error":false,"result":"","structured_output":{"matches":[
		{"key":"cf","confidence":0.9,"reason":"deploys workers"},
		{"key":"made-up","confidence":0.5,"reason":"hallucinated"}]}}`)
	picks, err := NewClaudeCode(bin, "").Rank(context.Background(), "where do I deploy", catalogLinks)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Key != "cf" {
		t.Errorf("picks = %+v, want only cf", picks)
	}
}

func TestClaudeCodeError(t *testing.T) {
	bin := fakeClaude(t, `{"is_error":true,"result":"Not logged in"}`)
	if _, err := NewClaudeCode(bin, "").Rank(context.Background(), "x", catalogLinks); err == nil {
		t.Error("expected error when CLI reports is_error")
	}
}
