//go:build darwin

package secret

import (
	"fmt"
	"os/exec"
	"strings"
)

// keychain stores secrets in the user's login Keychain through the `security`
// tool. Values go in over stdin (`security -i`), never on a command line.
type keychain struct{}

func systemBackend() backend { return keychain{} }

func service(name string) string { return "goli: " + name }

func (keychain) set(name, value string) error {
	cmd := exec.Command("/usr/bin/security", "-i")
	// Set() has already restricted value to [A-Za-z0-9_-], so quoting is safe.
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a goli -s %q -w %q\n", service(name), value))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("saving to Keychain failed: %w: %s", err, out)
	}
	if strings.Contains(string(out), "error") {
		return fmt.Errorf("saving to Keychain failed: %s", out)
	}
	return nil
}

func (keychain) get(name string) (string, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password", "-a", "goli", "-s", service(name), "-w").Output()
	v := strings.TrimSpace(string(out))
	if err != nil || v == "" {
		return "", ErrNotFound
	}
	return v, nil
}

func (keychain) del(name string) error {
	_ = exec.Command("/usr/bin/security", "delete-generic-password", "-a", "goli", "-s", service(name)).Run()
	return nil
}

func (keychain) where() string { return "your macOS Keychain" }
