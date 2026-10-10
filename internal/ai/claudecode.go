package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chetanparab/goli/internal/links"
)

// ClaudeCode ranks links by running the Claude Code CLI in print mode, so
// searches use the CLI's own login instead of an API key.
type ClaudeCode struct {
	bin      string
	model    string
	sendURLs string
}

// NewClaudeCode returns a ClaudeCode ranker. bin may be empty to search PATH
// and the usual install locations.
func NewClaudeCode(bin, model string) *ClaudeCode {
	return &ClaudeCode{bin: bin, model: model}
}

// Binary returns the path of the claude CLI, or an error if it can't be found.
func (c *ClaudeCode) Binary() (string, error) {
	if c.bin != "" {
		return c.bin, nil
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, ".claude", "local", "claude"),
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("claude CLI not found; install Claude Code or set ai.claude_bin")
}

// cleanEnv passes only what the CLI needs. Variables inherited from a parent
// Claude Code session (ANTHROPIC_BASE_URL, CLAUDE_CODE_*) would otherwise
// redirect or break the child's authentication.
func cleanEnv() []string {
	var env []string
	for _, k := range []string{"HOME", "USER", "LOGNAME", "PATH", "LANG", "TMPDIR", "SHELL"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// Rank asks the claude CLI which links match query.
func (c *ClaudeCode) Rank(ctx context.Context, query string, all []*links.Link) ([]Pick, error) {
	bin, err := c.Binary()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cat, known := catalog(all, c.sendURLs)
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	args := []string{
		"-p",
		"--output-format", "json",
		"--json-schema", string(schemaJSON),
		"--system-prompt", systemPrompt,
		"--tools", "", // no file, shell or web access: it only reads the catalog
		"--strict-mcp-config",   // and no MCP servers
		"--setting-sources", "", // ignore user/project settings and hooks
		"--no-session-persistence",
		"--effort", "low",
	}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader("Catalog:\n" + cat + "\nQuery: " + query)
	cmd.Dir = os.TempDir() // keep CLAUDE.md files from any project out of the prompt
	cmd.Env = cleanEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, runErr := cmd.Output()

	var res struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("claude CLI: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("parse claude CLI output: %w", err)
	}
	if res.IsError {
		return nil, fmt.Errorf("claude CLI: %s", res.Result)
	}
	data := res.StructuredOutput
	if len(data) == 0 || string(data) == "null" {
		data = []byte(res.Result)
	}
	var parsed struct {
		Matches []Pick `json:"matches"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse claude CLI result: %w", err)
	}
	return keepKnown(parsed.Matches, known), nil
}

// LoggedIn reports whether the claude CLI has credentials.
func (c *ClaudeCode) LoggedIn(ctx context.Context) (bool, error) {
	bin, err := c.Binary()
	if err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, bin, "auth", "status")
	cmd.Env = cleanEnv()
	out, err := cmd.Output()
	var st struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if jerr := json.Unmarshal(out, &st); jerr != nil {
		if err != nil {
			return false, err
		}
		return false, jerr
	}
	return st.LoggedIn, nil
}
