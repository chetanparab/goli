// Package links holds the link model and the YAML-backed store.
//
// links.yaml is a flat map from key to either a URL string or a full entry:
//
//	claude: https://claude.ai
//	gh:
//	  url: https://github.com/chetanparab
//	  desc: My GitHub
//	  tags: [code]
//
// Edits made through goli go through yaml.Node so comments and ordering
// in a hand-maintained file survive.
package links

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Variant is an alternative URL used when its conditions match the current context.
type Variant struct {
	When map[string]string `yaml:"when"`
	URL  string            `yaml:"url"`
}

// Link is a single go link.
type Link struct {
	Key      string    `yaml:"-" json:"key"`
	URL      string    `yaml:"url" json:"url"`
	Desc     string    `yaml:"desc,omitempty" json:"desc,omitempty"`
	Tags     []string  `yaml:"tags,omitempty" json:"tags,omitempty"`
	Variants []Variant `yaml:"variants,omitempty" json:"variants,omitempty"`
	// Aliases are other names that open this link: go/daily for go/standup.
	Aliases []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
}

// UnmarshalYAML accepts the short `key: url` form as well as a mapping.
func (l *Link) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		l.URL = n.Value
		return nil
	}
	type plain Link
	return n.Decode((*plain)(l))
}

func (l *Link) isShort() bool {
	return l.Desc == "" && len(l.Tags) == 0 && len(l.Variants) == 0 && len(l.Aliases) == 0
}

var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(/[a-z0-9._-]+)*$`)

// NormalizeKey lowercases a key and trims surrounding slashes.
func NormalizeKey(k string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(k)), "/")
}

// ValidateKey reports whether k is usable as a link key.
func ValidateKey(k string) error {
	if !keyRe.MatchString(k) {
		return fmt.Errorf("invalid key %q: use lowercase letters, digits, '.', '_', '-' and '/' separators", k)
	}
	return nil
}

// Validate checks a link before it is saved.
func (l *Link) Validate() error {
	if err := ValidateKey(l.Key); err != nil {
		return err
	}
	if err := validURL(l.URL); err != nil {
		return err
	}
	for _, v := range l.Variants {
		if err := validURL(v.URL); err != nil {
			return fmt.Errorf("variant: %w", err)
		}
	}
	seen := map[string]bool{l.Key: true}
	for i, a := range l.Aliases {
		a = NormalizeKey(a)
		if err := ValidateKey(a); err != nil {
			return fmt.Errorf("alias: %w", err)
		}
		if seen[a] {
			return fmt.Errorf("alias %q repeats the link's own name", a)
		}
		seen[a] = true
		l.Aliases[i] = a
	}
	return nil
}

func validURL(u string) error {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return fmt.Errorf("invalid url %q: must start with http:// or https://", u)
	}
	return nil
}

// Store is a links.yaml file, reloaded automatically when it changes on disk.
type Store struct {
	path  string
	mu    sync.RWMutex
	links map[string]*Link // by key
	named map[string]*Link // by key and by alias
	mtime time.Time
}

// Open loads the store at path, creating an empty file if needed.
func Open(path string) (*Store, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(emptyLinksYAML), 0o600); err != nil {
			return nil, err
		}
	}
	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Path returns the backing file.
func (s *Store) Path() string { return s.path }

func (s *Store) load() error {
	fi, err := os.Stat(s.path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	links, err := Parse(data)
	if err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	s.mu.Lock()
	// An edit() may have installed a newer copy while we were reading.
	if s.links == nil || fi.ModTime().After(s.mtime) {
		s.links, s.named, s.mtime = links, byName(links), fi.ModTime()
	}
	s.mu.Unlock()
	return nil
}

// byName indexes links under their keys and aliases. Parse has already
// rejected collisions.
func byName(links map[string]*Link) map[string]*Link {
	out := make(map[string]*Link, len(links))
	for k, l := range links {
		out[k] = l
		for _, a := range l.Aliases {
			out[a] = l
		}
	}
	return out
}

// Parse decodes links.yaml content.
func Parse(data []byte) (map[string]*Link, error) {
	raw := map[string]*Link{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]*Link, len(raw))
	for k, l := range raw {
		if l == nil {
			return nil, fmt.Errorf("link %q has no url", k)
		}
		l.Key = NormalizeKey(k)
		if err := l.Validate(); err != nil {
			return nil, err
		}
		if _, dup := out[l.Key]; dup {
			return nil, fmt.Errorf("duplicate key %q", l.Key)
		}
		out[l.Key] = l
	}
	// Aliases must not shadow a key or another link's alias.
	owner := map[string]string{}
	for _, l := range out {
		for _, a := range l.Aliases {
			if _, isKey := out[a]; isKey {
				return nil, fmt.Errorf("alias %q of %q is already a link", a, l.Key)
			}
			if o, dup := owner[a]; dup && o != l.Key {
				return nil, fmt.Errorf("alias %q belongs to both %q and %q", a, o, l.Key)
			}
			owner[a] = l.Key
		}
	}
	return out, nil
}

// refresh reloads the file if it changed since the last load. Errors keep the
// previous good copy so a half-saved edit never takes the server down.
func (s *Store) refresh() {
	fi, err := os.Stat(s.path)
	if err != nil {
		return
	}
	s.mu.RLock()
	same := fi.ModTime().Equal(s.mtime)
	s.mu.RUnlock()
	if !same {
		_ = s.load()
	}
}

// Get returns the link stored under key, which may be one of its aliases.
func (s *Store) Get(key string) (*Link, bool) {
	s.refresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.named[NormalizeKey(key)]
	return l, ok
}

// All returns every link sorted by key.
func (s *Store) All() []*Link {
	s.refresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Link, 0, len(s.links))
	for _, l := range s.links {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Put creates or replaces a link, preserving the rest of the file.
func (s *Store) Put(l *Link) error {
	l.Key = NormalizeKey(l.Key)
	if err := l.Validate(); err != nil {
		return err
	}
	return s.edit(func(root *yaml.Node) error {
		if o, ok := s.named[l.Key]; ok && o.Key != l.Key {
			return fmt.Errorf("%q is already an alias of %q", l.Key, o.Key)
		}
		val := &yaml.Node{}
		if l.isShort() {
			val = &yaml.Node{Kind: yaml.ScalarNode, Value: l.URL}
		} else if err := val.Encode(l); err != nil {
			return err
		}
		for i := 0; i+1 < len(root.Content); i += 2 {
			if NormalizeKey(root.Content[i].Value) == l.Key {
				root.Content[i+1] = val
				return nil
			}
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: l.Key}, val)
		return nil
	})
}

// Delete removes a link. It returns an error if the key doesn't exist.
func (s *Store) Delete(key string) error {
	key = NormalizeKey(key)
	return s.edit(func(root *yaml.Node) error {
		for i := 0; i+1 < len(root.Content); i += 2 {
			if NormalizeKey(root.Content[i].Value) == key {
				root.Content = append(root.Content[:i], root.Content[i+2:]...)
				return nil
			}
		}
		if l, ok := s.named[key]; ok {
			return fmt.Errorf("%q is an alias of %q: delete that, or edit its aliases", key, l.Key)
		}
		return fmt.Errorf("no link named %q", key)
	})
}

func (s *Store) edit(fn func(root *yaml.Node) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Serialise with other goli processes (CLI vs server) editing the same file.
	unlock, err := lockFile(s.path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.Kind == 0 || len(doc.Content) == 0 { // empty or comment-only file
		head := doc.HeadComment
		if head == "" {
			head = strings.TrimSpace(string(data))
		}
		doc = yaml.Node{Kind: yaml.DocumentNode, HeadComment: head,
			Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		if root.Kind == yaml.ScalarNode && root.Value == "" {
			root.Kind, root.Tag = yaml.MappingNode, ""
		} else {
			return fmt.Errorf("%s: top level must be a map of key: url", s.path)
		}
	}
	if err := fn(root); err != nil {
		return err
	}
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	links, err := Parse([]byte(b.String()))
	if err != nil {
		return err
	}
	if err := writeAtomic(s.path, []byte(b.String())); err != nil {
		return err
	}
	fi, err := os.Stat(s.path)
	if err != nil {
		return err
	}
	s.links, s.named, s.mtime = links, byName(links), fi.ModTime()
	return nil
}

// writeAtomic replaces path's contents via a temp file and rename, so a crash
// never leaves a half-written file. It writes through symlinks (dotfile and
// links-as-code setups) and keeps the file's permissions.
func writeAtomic(path string, data []byte) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".links-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if runtime.GOOS != "windows" { // Windows has no Unix permissions to keep
		if err := tmp.Chmod(mode); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

const emptyLinksYAML = `# goli links: key -> url, or key -> {url, desc, tags, variants}.
# Docs: https://github.com/chetanparab/goli#links-file
`
