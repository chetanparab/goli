//go:build linux

package secret

import (
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
	cmd := exec.Command(t.bin, "store", "--label=goli: "+name, "service", "goli", "key", name)
	cmd.Stdin = strings.NewReader(value) // never on the command line
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("secret-tool store: %w: %s", err, out)
	}
	return nil
}

func (t secretTool) get(name string) (string, error) {
	out, err := exec.Command(t.bin, "lookup", "service", "goli", "key", name).Output()
	v := strings.TrimSpace(string(out))
	if err != nil || v == "" {
		return "", ErrNotFound
	}
	return v, nil
}

func (t secretTool) del(name string) error {
	_ = exec.Command(t.bin, "clear", "service", "goli", "key", name).Run()
	return nil
}

func (t secretTool) where() string { return "your system keyring" }
