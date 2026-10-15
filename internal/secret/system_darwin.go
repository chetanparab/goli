//go:build darwin

package secret

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// keychain stores secrets in the user's login Keychain through the `security`
// tool. Values go in over stdin (`security -i`), never on a command line.
type keychain struct{}

func systemBackend() backend { return keychain{} }

func service(name string) string { return "goli: " + name }

const security = "/usr/bin/security"

func (keychain) set(name, value string) error {
	// Set() has already restricted value to [A-Za-z0-9_-], so quoting is safe.
	out, err := run(fmt.Sprintf("add-generic-password -U -a goli -s %q -w %q\n", service(name), value), security, "-i")
	if err != nil {
		return fmt.Errorf("saving to Keychain failed: %w", err)
	}
	if strings.Contains(string(out), "error") {
		return fmt.Errorf("saving to Keychain failed: %s", out)
	}
	return nil
}

// errItemNotFound is security's exit status for a missing item.
const errItemNotFound = 44

func (keychain) get(name string) (string, error) {
	out, err := run("", security, "find-generic-password", "-a", "goli", "-s", service(name), "-w")
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == errItemNotFound {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading the Keychain failed: %w", err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", ErrNotFound
	}
	return v, nil
}

func (keychain) del(name string) error {
	_, err := run("", security, "delete-generic-password", "-a", "goli", "-s", service(name))
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != errItemNotFound) {
		return fmt.Errorf("removing from Keychain failed: %w", err)
	}
	return nil
}

func (keychain) where() string { return "your macOS Keychain" }
