// Package config loads goli's settings and resolves its on-disk paths.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chetanparab/goli/internal/platform"
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
	// Names are the words typed before the slash: "go" makes go/standup work.
	// Several are fine ([go, g]); the first is the one goli shows. Each needs
	// a hosts-file line (`goli hosts`), and each is accepted as a Host header.
	Names []string `yaml:"names"`
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

// Dir returns goli's config directory: $GOLI_HOME, else ~/.config/goli
// (%AppData%\goli on Windows).
func Dir() (string, error) {
	if d := os.Getenv("GOLI_HOME"); d != "" {
		return d, nil
	}
	if runtime.GOOS == "windows" {
		d, err := os.UserConfigDir() // %AppData%
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "goli"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "goli"), nil
}

func defaults() Config {
	return Config{
		Listen:    platform.DefaultListen,
		Names:     []string{"go"},
		Hostnames: []string{"localhost", "127.0.0.1"},
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
		if err := os.WriteFile(path, []byte(fmt.Sprintf(defaultConfigYAML, platform.DefaultListen)), 0o600); err != nil {
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
	names, err := normalizeNames(cfg.Names)
	if err != nil {
		return nil, err
	}
	cfg.Names = names
	return &cfg, nil
}

// Dir is the directory this config was loaded from.
func (c *Config) Dir() string { return c.dir }

// nameRe is what a name before the slash may look like: a DNS label.
var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// normalizeNames lowercases, dedupes and validates names, defaulting to "go".
func normalizeNames(names []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" || seen[n] {
			continue
		}
		if !nameRe.MatchString(n) {
			return nil, fmt.Errorf("names: %q isn't usable before the slash (letters, digits and '-' only)", n)
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) == 0 {
		out = []string{"go"}
	}
	return out, nil
}

// LoadNames returns Names from config.yaml without creating anything, so
// `sudo goli hosts -apply` never writes files as root. Missing or unreadable
// config means the default, "go".
func LoadNames() []string {
	dir, err := Dir()
	if err != nil {
		return []string{"go"}
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return []string{"go"}
	}
	var c struct {
		Names []string `yaml:"names"`
	}
	if yaml.Unmarshal(data, &c) != nil {
		return []string{"go"}
	}
	names, err := normalizeNames(c.Names)
	if err != nil {
		return []string{"go"}
	}
	return names
}

// Name is the word goli shows before the slash: the first of Names.
func (c *Config) Name() string { return c.Names[0] }

// Hosts is every Host header the server should accept: Names plus Hostnames.
func (c *Config) Hosts() []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range append(append([]string{}, c.Names...), c.Hostnames...) {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

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
listen: "%s"
names: [go]                 # what you type before the slash; [go, g] gives you g/ too
hostnames: [localhost, 127.0.0.1]

# links_file: ~/code/goli-links/links.yaml

# Let your other devices use go/ too, e.g. your phone over Tailscale.
# Requests from anywhere else are still refused. Use your own tailnet name
# in hostnames (see 'tailscale status'), not a bare *.ts.net.
# allow: [tailscale]
# allow_write: false        # other devices can open and search, not edit
# hostnames: [localhost, 127.0.0.1, "go.tailXXXX.ts.net"]
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
