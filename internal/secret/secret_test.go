package secret

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileBackend(t *testing.T) {
	t.Setenv("GOLI_SECRET_BACKEND", "file")
	s := Store{Dir: t.TempDir()}
	if _, err := s.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty store: %v", err)
	}
	if err := s.Set("k", "sk-ant-api03-abc_DEF-123"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get("k"); err != nil || v != "sk-ant-api03-abc_DEF-123" {
		t.Fatalf("got %q %v", v, err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(s.Dir, "secrets", "k"))
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v", fi.Mode().Perm())
		}
	}
	if err := s.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}

func TestRejectsOddValues(t *testing.T) {
	t.Setenv("GOLI_SECRET_BACKEND", "file")
	s := Store{Dir: t.TempDir()}
	for _, bad := range []string{"", "short", `has"quote-xxxxxxxx`, "has space xxxxxxxx", "line\nbreak-xxxxxx"} {
		if err := s.Set("k", bad); err == nil {
			t.Errorf("Set(%q) should fail", bad)
		}
	}
}
