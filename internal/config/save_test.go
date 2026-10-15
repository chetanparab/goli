package config

import (
	"os"
	"strings"
	"testing"
)

func TestSetAIKeepsCommentsAndSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOLI_HOME", dir)
	cfg, err := Load() // writes the commented default file
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAI("anthropic", "claude-haiku-5-5"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfg.Path())
	s := string(data)
	for _, want := range []string{"# goli configuration", "listen:", "provider: anthropic", "model: claude-haiku-5-5", "enabled: true", "send_urls: host"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	again, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !again.AI.Enabled || again.AI.Provider != "anthropic" || again.AI.Model != "claude-haiku-5-5" {
		t.Errorf("reloaded %+v", again.AI)
	}
	if err := again.SetAI("off", ""); err != nil {
		t.Fatal(err)
	}
	off, _ := Load()
	if off.AI.Enabled || off.AI.Model != "" {
		t.Errorf("off: %+v", off.AI)
	}
}
