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

// system is the OS keyring, or nil when there isn't one (or
// GOLI_SECRET_BACKEND=file, used by tests).
func (s Store) system() backend {
	if os.Getenv("GOLI_SECRET_BACKEND") == "file" {
		return nil
	}
	return systemBackend()
}

func (s Store) file() fileBackend { return fileBackend{dir: filepath.Join(s.Dir, "secrets")} }

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
	if sys := s.system(); sys != nil {
		if err := sys.set(name, value); err == nil {
			_ = s.file().del(name) // the keyring copy wins; don't leave an old file behind
			return nil
		}
		// No usable keyring (e.g. a headless Linux box without a Secret
		// Service): fall back to the private file. Where() reports it.
	}
	return s.file().set(name, value)
}

// Get returns the value stored under name, or ErrNotFound.
func (s Store) Get(name string) (string, error) {
	var sysErr error
	if sys := s.system(); sys != nil {
		v, err := sys.get(name)
		if err == nil {
			return v, nil
		}
		sysErr = err
	}
	v, err := s.file().get(name)
	if errors.Is(err, ErrNotFound) && sysErr != nil && !errors.Is(sysErr, ErrNotFound) {
		return "", sysErr // a real keyring failure, not just a missing key
	}
	return v, err
}

// Delete removes the value stored under name everywhere it might be.
// Deleting a missing secret is not an error.
func (s Store) Delete(name string) error {
	var errs []error
	if sys := s.system(); sys != nil {
		errs = append(errs, sys.del(name))
	}
	errs = append(errs, s.file().del(name))
	return errors.Join(errs...)
}

// Where describes where the secret called name is kept (or would be), for
// status messages.
func (s Store) Where(name string) string {
	if _, err := s.file().get(name); err == nil || s.system() == nil {
		return s.file().where()
	}
	return s.system().where()
}

// ── file backend ────────────────────────────────────────────────────────────

type fileBackend struct{ dir string }

func (f fileBackend) path(name string) string { return filepath.Join(f.dir, name) }

func (f fileBackend) set(name, value string) error {
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return err
	}
	// Keep the folder out of git if the user versions their config directory.
	_ = os.WriteFile(filepath.Join(f.dir, ".gitignore"), []byte("*\n"), 0o600)
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
