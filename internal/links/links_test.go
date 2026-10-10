package links

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chetanparab/goli/internal/ctxinfo"
)

const fixture = `# my links
claude: https://claude.ai
gh:
  url: https://github.com/chetanparab
  desc: GitHub profile
jira: https://example.atlassian.net/browse/{1}
pr: https://github.com/{owner}/{repo}/pull/{n}
docs: https://docs.example.com/{*}
work/wiki: https://wiki.example.com
dash:
  url: https://prod.example.com
  variants:
    - when: {env: staging}
      url: https://staging.example.com
    - when: {hours: "22-06"}
      url: https://oncall.example.com
`

func newStore(t *testing.T, content string) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "links.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func noon() ctxinfo.Context {
	return ctxinfo.Context{Vars: map[string]string{}, Now: time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)}
}

func TestResolve(t *testing.T) {
	s := newStore(t, fixture)
	tests := []struct {
		path, query, want string
	}{
		{"claude", "", "https://claude.ai"},
		{"CLAUDE", "", "https://claude.ai"},
		{"gh/goli", "", "https://github.com/chetanparab/goli"},
		{"gh/goli/issues", "", "https://github.com/chetanparab/goli/issues"},
		{"jira/ABC-123", "", "https://example.atlassian.net/browse/ABC-123"},
		{"pr/acme/api/42", "", "https://github.com/acme/api/pull/42"},
		{"docs/a/b c", "", "https://docs.example.com/a/b%20c"},
		{"work/wiki", "", "https://wiki.example.com"},
		{"work/wiki/Page", "", "https://wiki.example.com/Page"},
		{"claude", "q=hi&ctx.env=x", "https://claude.ai?q=hi"},
		{"dash", "", "https://prod.example.com"},
	}
	for _, tt := range tests {
		q, _ := url.ParseQuery(tt.query)
		res, err := s.Resolve(tt.path, q, noon())
		if err != nil {
			t.Errorf("Resolve(%q): %v", tt.path, err)
			continue
		}
		if res.URL != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.path, res.URL, tt.want)
		}
	}
}

func TestResolveContext(t *testing.T) {
	s := newStore(t, fixture)
	c := noon()
	c.Vars["env"] = "staging"
	if res, _ := s.Resolve("dash", nil, c); res.URL != "https://staging.example.com" {
		t.Errorf("env=staging: got %s", res.URL)
	}
	late := ctxinfo.Context{Now: time.Date(2026, 10, 7, 23, 30, 0, 0, time.Local)}
	if res, _ := s.Resolve("dash", nil, late); res.URL != "https://oncall.example.com" {
		t.Errorf("23:30: got %s", res.URL)
	}
}

func TestResolveErrors(t *testing.T) {
	s := newStore(t, fixture)
	if _, err := s.Resolve("nope", nil, noon()); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	var missing *MissingArgsError
	if _, err := s.Resolve("pr/acme", nil, noon()); !errors.As(err, &missing) {
		t.Fatalf("want MissingArgsError, got %v", err)
	}
	if strings.Join(missing.Need, ",") != "repo,n" {
		t.Errorf("missing = %v", missing.Need)
	}
}

func TestPutPreservesComments(t *testing.T) {
	s := newStore(t, fixture)
	if err := s.Put(&Link{Key: "new", URL: "https://new.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(&Link{Key: "claude", URL: "https://claude.ai/new", Desc: "Claude"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("work/wiki"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(s.Path())
	if !strings.Contains(string(data), "# my links") {
		t.Errorf("comment lost:\n%s", data)
	}
	if l, _ := s.Get("claude"); l.URL != "https://claude.ai/new" || l.Desc != "Claude" {
		t.Errorf("update failed: %+v", l)
	}
	if _, ok := s.Get("work/wiki"); ok {
		t.Error("delete failed")
	}
	if _, ok := s.Get("new"); !ok {
		t.Error("add failed")
	}
	if err := s.Delete("missing"); err == nil {
		t.Error("deleting a missing key should fail")
	}
}

func TestPutIntoEmptyFile(t *testing.T) {
	s := newStore(t, "# only a comment\n")
	if err := s.Put(&Link{Key: "a", URL: "https://a.example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("a"); !ok {
		t.Error("link not saved")
	}
	path := filepath.Join(t.TempDir(), "sub", "links.yaml")
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.Put(&Link{Key: "b", URL: "https://b.example.com"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidation(t *testing.T) {
	s := newStore(t, fixture)
	bad := []*Link{
		{Key: "Has Space", URL: "https://x.com"},
		{Key: "_reserved", URL: "https://x.com"},
		{Key: "ok", URL: "javascript:alert(1)"},
		{Key: "ok", URL: "ftp://x.com"},
	}
	for _, l := range bad {
		if err := s.Put(l); err == nil {
			t.Errorf("Put(%+v) should fail", l)
		}
	}
	if _, err := Parse([]byte("a: https://x.com\nA: https://y.com\n")); err == nil {
		t.Error("duplicate keys differing in case should fail")
	}
}

func TestReloadOnChange(t *testing.T) {
	s := newStore(t, fixture)
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(s.Path(), []byte("fresh: https://fresh.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Second)
	os.Chtimes(s.Path(), future, future)
	if _, ok := s.Get("fresh"); !ok {
		t.Error("store did not reload")
	}
}

func TestExampleFileParses(t *testing.T) {
	data, err := os.ReadFile("../../examples/links.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(data); err != nil {
		t.Fatal(err)
	}
}

func TestResolveEscapingByPart(t *testing.T) {
	s := newStore(t, `search: https://docs.example.com/search?q={*}
team: https://{1}.slack.example.com/archives
sorted: https://x.example.com/?b=1&a=2
spa: https://app.example.com/#/home
`)
	tests := []struct{ path, query, want string }{
		{"search/c++", "", "https://docs.example.com/search?q=c%2B%2B"},
		{"search/a&admin=1", "", "https://docs.example.com/search?q=a%26admin%3D1"},
		{"team/acme", "", "https://acme.slack.example.com/archives"},
		{"sorted", "z=1", "https://x.example.com/?b=1&a=2&z=1"},
		{"spa/settings", "", "https://app.example.com/#/home/settings"},
	}
	for _, tt := range tests {
		q, _ := url.ParseQuery(tt.query)
		res, err := s.Resolve(tt.path, q, noon())
		if err != nil {
			t.Errorf("Resolve(%q): %v", tt.path, err)
			continue
		}
		if res.URL != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.path, res.URL, tt.want)
		}
	}
	for _, bad := range []string{"team/evil.com:443@x", "team/a.b"} {
		if _, err := s.Resolve(bad, nil, noon()); err == nil {
			t.Errorf("Resolve(%q) should refuse a host injection", bad)
		}
	}
}

func TestPutWritesThroughSymlinkAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "repo", "links.yaml")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte("a: https://a.example.com\n"), 0o640)
	link := filepath.Join(dir, "links.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	s, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(&Link{Key: "b", URL: "https://b.example.com"}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced by a regular file")
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode changed to %v", fi.Mode().Perm())
	}
	if data, _ := os.ReadFile(target); !strings.Contains(string(data), "b.example.com") {
		t.Error("edit didn't reach the symlink target")
	}
}
