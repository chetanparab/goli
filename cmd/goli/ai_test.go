package main

import (
	"bufio"
	"strings"
	"testing"

	"github.com/chetanparab/goli/internal/ai"
	"github.com/chetanparab/goli/internal/config"
)

// Piped setup input: the key is read from the same reader as the menu
// answers, so lines after it (the model choice) aren't lost.
func TestPromptKeyReadsSharedReader(t *testing.T) {
	t.Setenv("GOLI_HOME", t.TempDir())
	t.Setenv("GOLI_SECRET_BACKEND", "file")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	in := bufio.NewReader(strings.NewReader("sk-ant-api03-piped_KEY-1\n2\n"))
	if err := promptKey(cfg, in); err != nil {
		t.Fatal(err)
	}
	if v, err := cfg.Keys().Get(ai.APIKeyName); err != nil || v != "sk-ant-api03-piped_KEY-1" {
		t.Fatalf("saved %q %v", v, err)
	}
	if rest, _ := in.ReadString('\n'); rest != "2\n" {
		t.Errorf("next answer lost: %q", rest)
	}
}

func TestModelFor(t *testing.T) {
	for _, c := range []struct{ provider, name, want string }{
		{ai.ProviderAnthropic, "haiku", "claude-haiku-5-5"},
		{ai.ProviderAnthropic, "Opus", "claude-opus-5-5"},
		{ai.ProviderAnthropic, "claude-sonnet-5-5", "claude-sonnet-5-5"},
		{ai.ProviderClaudeCode, "sonnet", "sonnet"},
	} {
		if got, err := modelFor(c.provider, c.name); err != nil || got != c.want {
			t.Errorf("modelFor(%s, %s) = %q, %v", c.provider, c.name, got, err)
		}
	}
	for _, bad := range []string{"", "5", "o", "5.5"} {
		if _, err := modelFor(ai.ProviderAnthropic, bad); err == nil {
			t.Errorf("modelFor(api, %q) should fail", bad)
		}
	}
}
