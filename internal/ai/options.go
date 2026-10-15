package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/links"
	"github.com/chetanparab/goli/internal/secret"
)

// Option describes one way to power AI search and whether it's ready to use.
type Option struct {
	ID     string // ProviderOff, ProviderClaudeCode or ProviderAnthropic
	Name   string
	Blurb  string
	Ready  bool
	Detail string // what was found, or what to do next
	Active bool
}

// KeySource reports where an API key would come from, or "" if there is none.
func KeySource(keys secret.Store) string {
	if _, err := keys.Get(APIKeyName); err == nil {
		return "saved in " + keys.Where(APIKeyName)
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		// Only this shell has it; the background server started at login doesn't.
		return "from ANTHROPIC_API_KEY in this shell only — run `goli ai key` so the background server can use it"
	}
	if home, err := os.UserHomeDir(); err == nil {
		if entries, err := os.ReadDir(filepath.Join(home, ".config", "anthropic", "credentials")); err == nil && len(entries) > 0 {
			return "from your `ant auth login` profile"
		}
	}
	return ""
}

// Options lists every way to power AI search, with live readiness checks.
func Options(ctx context.Context, cfg config.AI, keys secret.Store) []Option {
	active := Active(cfg)

	cc := Option{ID: ProviderClaudeCode, Name: "Claude Code",
		Blurb: "Uses the Claude Code app you're already signed in to — your Claude subscription. No API key."}
	code := NewClaudeCode(cfg.ClaudeBin, "")
	if _, err := code.Binary(); err != nil {
		cc.Detail = "Claude Code isn't installed. Get it at claude.com/claude-code, then run `claude auth login`."
	} else {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		in, err := code.LoggedIn(cctx)
		cancel()
		switch {
		case in:
			cc.Ready, cc.Detail = true, "Installed and signed in."
		case err != nil:
			cc.Detail = "Couldn't check the Claude Code login: " + err.Error()
		default:
			cc.Detail = "Installed but not signed in. Run `claude auth login` in a terminal."
		}
	}

	api := Option{ID: ProviderAnthropic, Name: "Claude API key",
		Blurb: "Uses an API key from console.anthropic.com — pay per search (a fraction of a cent), free credits count."}
	if src := KeySource(keys); src != "" {
		api.Ready, api.Detail = true, "Key "+src+"."
	} else {
		api.Detail = "No key yet. Create one at console.anthropic.com → API keys, then paste it here."
	}

	off := Option{ID: ProviderOff, Name: "Off", Ready: true,
		Blurb:  "Built-in search that forgives typos and matches descriptions. Nothing leaves your computer.",
		Detail: "Always available."}

	opts := []Option{off, cc, api}
	for i := range opts {
		opts[i].Active = opts[i].ID == active
	}
	return opts
}

// sample is a tiny made-up catalog for Test, so testing never sends the
// user's real links anywhere.
var sample = []*links.Link{
	{Key: "standup", URL: "https://meet.example.com/daily", Desc: "Daily stand-up video call"},
	{Key: "payroll", URL: "https://hr.example.com/pay", Desc: "Monthly salary statements"},
	{Key: "roadmap", URL: "https://docs.example.com/roadmap", Desc: "Product roadmap"},
}

// Test runs one search against a made-up catalog and checks the answer.
func Test(ctx context.Context, r Ranker) (time.Duration, error) {
	if r == nil {
		return 0, errors.New("AI search is off")
	}
	start := time.Now()
	picks, err := r.Rank(ctx, "join my morning team meeting", sample)
	took := time.Since(start)
	if err != nil {
		return took, err
	}
	if len(picks) == 0 || picks[0].Key != "standup" {
		return took, fmt.Errorf("it answered, but picked %v instead of standup", picks)
	}
	return took, nil
}
