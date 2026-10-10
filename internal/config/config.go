// Package config loads goli's settings and resolves its on-disk paths.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Network describes how to recognise a named network for context-aware links.
// A network matches when any of its non-empty fields matches.
type Network struct {
	Gateway  string `yaml:"gateway,omitempty"`  // default gateway IP, e.g. 192.168.1.1
	Resolves string `yaml:"resolves,omitempty"` // a hostname that only resolves on this network (VPN, office)
}

// AI configures semantic lookup.
type AI struct {
	Enabled bool `yaml:"enabled"`
	// Provider is "claude-code" (the local claude CLI and its login) or
	// "anthropic" (the Claude API).
	Provider  string `yaml:"provider,omitempty"`
	Model     string `yaml:"model,omitempty"`      // empty: the provider's default
	ClaudeBin string `yaml:"claude_bin,omitempty"` // path to the claude CLI, if not on PATH
	// SendURLs controls how much of each link's URL is shared with the AI
	// provider: "host" (default, e.g. docs.example.com), "full", or "none".
	SendURLs string `yaml:"send_urls,omitempty"`
}

// Config is the contents of config.yaml.
type Config struct {
	// Listen is the address the server binds. See README for why the default is ":80".
	Listen string `yaml:"listen"`
	// Hostnames are the Host header values the server accepts (DNS-rebinding
	// protection). An entry like "*.ts.net" matches any subdomain.
	Hostnames []string `yaml:"hostnames"`
	// Allow lists extra client networks besides this machine: CIDRs, IPs, or
	// "tailscale" for your tailnet (100.64.0.0/10 and fd7a:115c:a1e0::/48).
	Allow []string `yaml:"allow,omitempty"`
	// AllowWrite lets clients from Allow networks create and delete links.
	// Off by default: other devices can open and search links, not change them.
	AllowWrite bool `yaml:"allow_write,omitempty"`
	// KeepAwake stops the computer from idle-sleeping while plugged in, so
	// other devices can always reach it (macOS only).
	KeepAwake bool `yaml:"keep_awake,omitempty"`
	// LinksFile overrides the location of links.yaml.
	LinksFile string             `yaml:"links_file,omitempty"`
	AI        AI                 `yaml:"ai"`
	Networks  map[string]Network `yaml:"networks,omitempty"`

	dir string
}

// Dir returns goli's config directory: $GOLI_HOME, else ~/.config/goli.
func Dir() (string, error) {
	if d := os.Getenv("GOLI_HOME"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "goli"), nil
}

func defaults() Config {
	return Config{
		Listen:    ":80",
		Hostnames: []string{"go", "localhost", "127.0.0.1"},
		AI:        AI{Enabled: false, Provider: "claude-code", SendURLs: "host"},
	}
}

// Load reads config.yaml, writing a commented default if it doesn't exist.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	cfg := defaults()
	cfg.dir = dir
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte(defaultConfigYAML), 0o600); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // catch typos like "alow:" instead of ignoring them
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, nil
}

// Dir is the directory this config was loaded from.
func (c *Config) Dir() string { return c.dir }

// LinksPath is the resolved path of links.yaml.
func (c *Config) LinksPath() string {
	if c.LinksFile != "" {
		return expandHome(c.LinksFile)
	}
	return filepath.Join(c.dir, "links.yaml")
}

// ContextPath is where `goli ctx set` persists context variables.
func (c *Config) ContextPath() string { return filepath.Join(c.dir, "context.yaml") }

func expandHome(p string) string {
	if len(p) > 1 && p[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

const defaultConfigYAML = `# goli configuration. See https://github.com/chetanparab/goli#configuration
listen: ":80"
hostnames: [go, localhost, 127.0.0.1]

# links_file: ~/code/goli-links/links.yaml

# Let your other devices use go/ too, e.g. your phone over Tailscale.
# Requests from anywhere else are still refused. Use your own tailnet name
# in hostnames (see 'tailscale status'), not a bare *.ts.net.
# allow: [tailscale]
# allow_write: false        # other devices can open and search, not edit
# hostnames: [go, localhost, 127.0.0.1, "go.tailXXXX.ts.net"]
# keep_awake: true          # don't idle-sleep while plugged in (macOS)

# AI search (go/?words) is off until you turn it on. When on, each search
# sends your query and link catalog (keys, descriptions, tags, and URLs as
# set by send_urls) to the provider.
ai:
  enabled: false
  # claude-code: runs the local claude CLI with its own login (run 'claude auth login' once)
  # anthropic:   calls the Claude API (ANTHROPIC_API_KEY or 'ant auth login')
  provider: claude-code
  send_urls: host            # host | full | none
  # model: claude-opus-5-5   # optional; defaults to the provider's default

# Named networks for context-aware links (variants with when: {network: home}).
# networks:
#   home:   {gateway: 192.168.1.1}
#   office: {resolves: intranet.example.com}
`
