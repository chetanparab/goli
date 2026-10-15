package config

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestSetAIWritesThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra rights on Windows")
	}
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(target, []byte("# mine\nlisten: \":8080\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOLI_HOME", home)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAI("off", ""); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(home, "config.yaml")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v", err)
	}
	if b, _ := os.ReadFile(target); !strings.Contains(string(b), "# mine") || !strings.Contains(string(b), "ai:") {
		t.Errorf("target not updated:\n%s", b)
	}
}
