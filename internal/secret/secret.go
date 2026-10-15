// Package secret keeps small secrets (an Anthropic API key) out of config
// files: in the macOS Keychain, the Linux Secret Service (via secret-tool), or
// otherwise a 0600 file in goli's config directory.
package secret

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrNotFound means no secret is stored under that name.
var ErrNotFound = errors.New("secret not found")

// Store saves secrets for goli. Dir is goli's config directory (used by the
// file backend).
type Store struct {
	Dir string
}

// backend picks where secrets live. GOLI_SECRET_BACKEND=file forces the file
// backend (used by tests).
func (s Store) backend() backend {
	if os.Getenv("GOLI_SECRET_BACKEND") != "file" {
		if b := systemBackend(); b != nil {
			return b
		}
	}
	return fileBackend{dir: filepath.Join(s.Dir, "secrets")}
}

type backend interface {
	set(name, value string) error
	get(name string) (string, error)
	del(name string) error
	where() string
}

// safe is what a stored value may contain: API keys are letters, digits, '-' and '_'.
var safe = regexp.MustCompile(`^[A-Za-z0-9_\-]{8,512}$`)

// Set stores value under name, replacing any previous value.
func (s Store) Set(name, value string) error {
	value = strings.TrimSpace(value)
	if !safe.MatchString(value) {
		return errors.New("that doesn't look like a key (expected letters, digits, '-' and '_')")
	}
	return s.backend().set(name, value)
}

// Get returns the value stored under name, or ErrNotFound.
func (s Store) Get(name string) (string, error) { return s.backend().get(name) }

// Delete removes the value stored under name. Deleting a missing secret is not an error.
func (s Store) Delete(name string) error { return s.backend().del(name) }

// Where describes where secrets are kept, for status messages.
func (s Store) Where() string { return s.backend().where() }

// ── file backend ────────────────────────────────────────────────────────────

type fileBackend struct{ dir string }

func (f fileBackend) path(name string) string { return filepath.Join(f.dir, name) }

func (f fileBackend) set(name, value string) error {
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(f.path(name), []byte(value+"\n"), 0o600)
}

func (f fileBackend) get(name string) (string, error) {
	data, err := os.ReadFile(f.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", ErrNotFound
	}
	return v, nil
}

func (f fileBackend) del(name string) error {
	if err := os.Remove(f.path(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (f fileBackend) where() string { return "a private file in " + f.dir }
