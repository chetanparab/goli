//go:build linux

package secret

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func systemBackend() backend {
	if bin, err := exec.LookPath("secret-tool"); err == nil {
		return secretTool{bin: bin}
	}
	return nil
}

// ── Linux Secret Service (secret-tool) ──────────────────────────────────────

type secretTool struct{ bin string }

func (t secretTool) set(name, value string) error {
	if _, err := run(value, t.bin, "store", "--label=goli: "+name, "service", "goli", "key", name); err != nil {
		return fmt.Errorf("secret-tool store: %w", err)
	}
	return nil
}

func (t secretTool) get(name string) (string, error) {
	out, err := run("", t.bin, "lookup", "service", "goli", "key", name)
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return "", fmt.Errorf("secret-tool lookup: %w", err) // timed out or couldn't start
	}
	// secret-tool exits 1 with no output both for "not found" and for "no
	// Secret Service"; Store.Get then checks the file fallback.
	v := strings.TrimSpace(string(out))
	if err != nil || v == "" {
		return "", ErrNotFound
	}
	return v, nil
}

func (t secretTool) del(name string) error {
	_, _ = run("", t.bin, "clear", "service", "goli", "key", name) // missing is fine
	return nil
}

func (t secretTool) where() string { return "your system keyring" }
