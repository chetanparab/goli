// Package config loads goli's settings and resolves its on-disk paths.
package config

import (
	"errors"
	"fmt"
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
	Enabled bool   `yaml:"enabled"`
	Model   string `yaml:"model,omitempty"`
}

// Config is the contents of config.yaml.
type Config struct {
	// Listen is the address the server binds. See README for why the default is ":80".
	Listen string `yaml:"listen"`
	// Hostnames are the Host header values the server accepts (DNS-rebinding protection).
	Hostnames []string `yaml:"hostnames"`
	// LinksFile overrides the location of links.yaml.
	LinksFile string             `yaml:"links_file,omitempty"`
	AI        AI                 `yaml:"ai"`
	Networks  map[string]Network `yaml:"networks,omitempty"`

	dir string
}

// DefaultModel is the Claude model used for semantic lookup unless configured.
const DefaultModel = "claude-opus-5-5"

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
		AI:        AI{Enabled: true, Model: DefaultModel},
	}
}

// Load reads config.yaml, writing a commented default if it doesn't exist.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cfg := defaults()
	cfg.dir = dir
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte(defaultConfigYAML), 0o644); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.AI.Model == "" {
		cfg.AI.Model = DefaultModel
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

ai:
  enabled: true            # needs ANTHROPIC_API_KEY or an 'ant auth login' profile
  model: claude-opus-5-5

# Named networks for context-aware links (variants with when: {network: home}).
# networks:
#   home:   {gateway: 192.168.1.1}
#   office: {resolves: intranet.example.com}
`
