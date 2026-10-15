package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/chetanparab/goli/internal/ai"
	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/platform"
)

const aiUsage = `goli ai — choose how AI search (go/?words) is powered.

  goli ai                      show the options and which one is in use
  goli ai setup                guided setup: pick an option, add a key, test it
  goli ai use off              built-in search only; nothing leaves your computer
  goli ai use claude-code      your Claude Code login (Claude subscription)
  goli ai use api [model]      a Claude API key (models: opus, sonnet, haiku)
  goli ai key                  save an API key (typed hidden, kept in your keychain)
  goli ai key --remove         forget the saved API key
  goli ai test                 run one search on made-up links to check it works

The same choices are in goli's web page on this computer: http://go/_/settings
`

func cmdAI(cfg *config.Config, args []string) error {
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "", "status":
		return aiStatus(cfg)
	case "help", "-h", "--help":
		fmt.Print(aiUsage)
		return nil
	case "setup":
		return aiSetup(cfg)
	case "use":
		if len(args) == 0 {
			return errors.New("usage: goli ai use off|claude-code|api [opus|sonnet|haiku]")
		}
		provider := ai.NormalizeProvider(args[0])
		if err := checkProvider(provider); err != nil {
			return err
		}
		model := ""
		if len(args) > 1 {
			m, err := modelFor(provider, args[1])
			if err != nil {
				return err
			}
			model = m
		}
		return aiApply(cfg, provider, model, true)
	case "key":
		if len(args) > 0 && (args[0] == "--remove" || args[0] == "-remove") {
			if err := cfg.Keys().Delete(ai.APIKeyName); err != nil {
				return err
			}
			fmt.Println("Saved API key removed.")
			notifyServer()
			return nil
		}
		if err := promptKey(cfg, bufio.NewReader(os.Stdin)); err != nil {
			return err
		}
		notifyServer()
		return nil
	case "test":
		return aiTest(cfg)
	}
	return fmt.Errorf("unknown command %q\n\n%s", sub, aiUsage)
}

func aiStatus(cfg *config.Config) error {
	fmt.Println("AI search options:")
	for _, o := range ai.Options(context.Background(), cfg.AI, cfg.Keys()) {
		mark := "  "
		if o.Active {
			mark = "▸ "
		}
		ready := "ready"
		if !o.Ready {
			ready = "needs setup"
		}
		fmt.Printf("%s%-15s %-12s %s\n", mark, o.Name, "("+ready+")", o.Detail)
	}
	fmt.Println("\nChange it with `goli ai setup`, or at http://go/_/settings on this computer.")
	return nil
}

func aiSetup(cfg *config.Config) error {
	in := bufio.NewReader(os.Stdin)
	opts := ai.Options(context.Background(), cfg.AI, cfg.Keys())
	fmt.Println("How should goli power AI search (go/?words)?")
	fmt.Println()
	for i, o := range opts {
		status := "ready"
		if !o.Ready {
			status = "needs setup"
		}
		active := ""
		if o.Active {
			active = "  ← in use"
		}
		fmt.Printf("  %d) %s%s\n     %s\n     %s — %s\n\n", i+1, o.Name, active, o.Blurb, status, o.Detail)
	}
	choice := ask(in, fmt.Sprintf("Choose 1-%d", len(opts)), "")
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(opts) {
		return errors.New("no change made")
	}
	o := opts[n-1]
	model := ""
	switch o.ID {
	case ai.ProviderClaudeCode:
		if !o.Ready {
			fmt.Println("\nClaude Code isn't ready yet:", o.Detail)
			fmt.Println("Do that, then run `goli ai setup` again.")
			return nil
		}
		model = chooseModel(in, ai.ClaudeCodeModels)
	case ai.ProviderAnthropic:
		if !o.Ready || strings.EqualFold(ask(in, "Save a new API key now? [y/N]", "n"), "y") {
			if err := promptKey(cfg, in); err != nil {
				return err
			}
		}
		model = chooseModel(in, ai.APIModels)
		if model == ai.DefaultAPIModel {
			model = ""
		}
	}
	if err := aiApply(cfg, o.ID, model, false); err != nil {
		return err
	}
	if o.ID != ai.ProviderOff {
		fmt.Println("\nTesting with three made-up links (your own links aren't sent)…")
		return aiTest(cfg)
	}
	return nil
}

func chooseModel(in *bufio.Reader, models []ai.Model) string {
	fmt.Println("\nWhich model?")
	for i, m := range models {
		fmt.Printf("  %d) %s — %s\n", i+1, m.Name, m.Note)
	}
	n, err := strconv.Atoi(ask(in, "Choose", "1"))
	if err != nil || n < 1 || n > len(models) {
		n = 1
	}
	return models[n-1].ID
}

func ask(in *bufio.Reader, prompt, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// promptKey reads an API key without echoing it and saves it to the keychain.
// When stdin isn't a terminal (a pipe), it reads the next line from in, the
// same reader the rest of setup uses, so buffered input isn't lost.
func promptKey(cfg *config.Config, in *bufio.Reader) error {
	fmt.Println("Create a key at https://console.anthropic.com/settings/keys, then paste it here.")
	var key string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("API key (hidden): ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		key = string(b)
	} else {
		fmt.Fprintln(os.Stderr, "goli: not a terminal, so the key is read from input as-is (it isn't hidden)")
		fmt.Print("API key: ")
		key, _ = in.ReadString('\n')
	}
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, "sk-ant-") {
		return errors.New("that doesn't look like an Anthropic API key (they start with sk-ant-)")
	}
	if err := cfg.Keys().Set(ai.APIKeyName, key); err != nil {
		return err
	}
	fmt.Println("Saved in", cfg.Keys().Where(ai.APIKeyName)+".")
	return nil
}

// modelFor accepts a model ID or the first word of its name ("haiku").
func modelFor(provider, name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch provider {
	case ai.ProviderAnthropic:
		for _, m := range ai.APIModels {
			first, _, _ := strings.Cut(strings.ToLower(m.Name), " ")
			if name != "" && (m.ID == name || first == name) {
				return m.ID, nil
			}
		}
	case ai.ProviderClaudeCode:
		for _, m := range ai.ClaudeCodeModels {
			if m.ID != "" && m.ID == name {
				return m.ID, nil
			}
		}
	}
	return "", fmt.Errorf("unknown model %q for %s", name, provider)
}

func checkProvider(provider string) error {
	switch provider {
	case ai.ProviderOff, ai.ProviderClaudeCode, ai.ProviderAnthropic:
		return nil
	}
	return fmt.Errorf("unknown option %q (use off, claude-code or api)", provider)
}

func aiApply(cfg *config.Config, provider, model string, test bool) error {
	if err := checkProvider(provider); err != nil {
		return err
	}
	if err := cfg.SetAI(provider, model); err != nil {
		return err
	}
	name := map[string]string{ai.ProviderOff: "off", ai.ProviderClaudeCode: "Claude Code", ai.ProviderAnthropic: "the Claude API"}[provider]
	fmt.Println("AI search now uses", name+".")
	notifyServer()
	if test && provider != ai.ProviderOff {
		return aiTest(cfg) // the choice is saved either way; a failed test still exits nonzero
	}
	return nil
}

func aiTest(cfg *config.Config) error {
	r, err := ai.New(cfg.AI, cfg.Keys())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	took, err := ai.Test(ctx, r)
	if err != nil {
		msg, _, _ := strings.Cut(err.Error(), "\n")
		fmt.Println("✗ Test failed:", msg)
		return errReported
	}
	fmt.Printf("✓ It works — found the right link in %.1fs.\n", took.Seconds())
	return nil
}

// notifyServer restarts the background server so it picks up a new choice or
// key. (Changes made in the web settings page apply instantly on their own.)
func notifyServer() {
	if !platform.ServiceInstalled() {
		return
	}
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://127.0.0.1" + portSuffix() + "/_/health")
	if err != nil {
		return
	}
	resp.Body.Close()
	if err := platform.RestartService(); err == nil {
		fmt.Println("(restarted the background server so it uses this)")
	}
}

func portSuffix() string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	if i := strings.LastIndex(cfg.Listen, ":"); i >= 0 && cfg.Listen[i+1:] != "80" {
		return ":" + cfg.Listen[i+1:]
	}
	return ""
}
