// Package usage counts how often each link is opened, on this computer only,
// so search can rank the links you actually use first and `goli ls -unused`
// can show the ones you don't. Nothing here is ever sent anywhere.
package usage

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/chetanparab/goli/internal/fslock"
)

// Entry is one link's history.
type Entry struct {
	Count int       `json:"count"`
	Last  time.Time `json:"last"`
}

// Store is usage.json in goli's config directory.
type Store struct {
	path string
	mu   sync.Mutex
	// cache is the last read copy, refreshed when the file changes.
	cache map[string]Entry
	mtime time.Time
}

// Open returns the store for dir (nothing is read until it's needed).
func Open(dir string) *Store { return &Store{path: filepath.Join(dir, "usage.json")} }

func (s *Store) read() map[string]Entry {
	fi, err := os.Stat(s.path)
	if err != nil {
		return map[string]Entry{}
	}
	if s.cache != nil && fi.ModTime().Equal(s.mtime) {
		return s.cache
	}
	data, err := os.ReadFile(s.path)
	m := map[string]Entry{}
	if err == nil {
		_ = json.Unmarshal(data, &m) // a corrupt file just means starting over
	}
	s.cache, s.mtime = m, fi.ModTime()
	return m
}

// All returns every link's history, by key.
func (s *Store) All() map[string]Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Entry, len(s.read()))
	for k, v := range s.read() {
		out[k] = v
	}
	return out
}

// Record notes one open of key. Errors are swallowed: a redirect must never
// fail because a counter couldn't be written.
func (s *Store) Record(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := fslock.Lock(s.path + ".lock")
	if err != nil {
		return
	}
	defer unlock()
	s.cache = nil // re-read under the lock
	m := s.read()
	e := m[key]
	e.Count++
	e.Last = time.Now()
	m[key] = e
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, s.path)
	}
	s.cache = nil
}

// Forget drops key's history (after a link is deleted or renamed).
func (s *Store) Forget(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := fslock.Lock(s.path + ".lock")
	if err != nil {
		return
	}
	defer unlock()
	s.cache = nil
	m := s.read()
	if _, ok := m[key]; !ok {
		return
	}
	delete(m, key)
	if data, err := json.MarshalIndent(m, "", "  "); err == nil {
		tmp := s.path + ".tmp"
		if os.WriteFile(tmp, data, 0o600) == nil {
			_ = os.Rename(tmp, s.path)
		}
	}
	s.cache = nil
}

// Boost returns a small ranking bonus (0 to about 0.12) for links opened
// often and recently: enough to break ties and lift daily links above
// similar names, not enough to beat a clearly better text match.
func (s *Store) Boost() func(key string) float64 {
	all := s.All()
	now := time.Now()
	return func(key string) float64 {
		e, ok := all[key]
		if !ok || e.Count == 0 {
			return 0
		}
		freq := math.Log1p(float64(e.Count)) / math.Log1p(50) // 50 opens ≈ full marks
		days := now.Sub(e.Last).Hours() / 24
		recency := math.Exp(-days / 30) // halves roughly every three weeks
		return 0.12 * math.Min(1, freq) * (0.4 + 0.6*recency)
	}
}

// ErrNoHistory means usage.json doesn't exist yet, so "unused" is meaningless.
var ErrNoHistory = errors.New("no usage history yet")

// Unused returns, sorted, the keys among candidates that were never opened
// or not opened since the cutoff.
func (s *Store) Unused(candidates []string, since time.Time) ([]string, error) {
	if _, err := os.Stat(s.path); err != nil {
		return nil, ErrNoHistory
	}
	all := s.All()
	var out []string
	for _, k := range candidates {
		if e, ok := all[k]; !ok || e.Last.Before(since) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}
