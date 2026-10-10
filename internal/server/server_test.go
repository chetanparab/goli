package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/links"
)

func newServer(t *testing.T) (*Server, *links.Store) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GOLI_HOME", dir)
	path := filepath.Join(dir, "links.yaml")
	os.WriteFile(path, []byte("claude: https://claude.ai\nroadmap:\n  url: https://docs.example.com/roadmap\n  desc: Product roadmap\n"), 0o644)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AI.Enabled = false
	store, err := links.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil))), store
}

func do(h http.Handler, method, target string, body io.Reader, mod func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	req.Host = "go"
	req.RemoteAddr = "127.0.0.1:5555"
	if mod != nil {
		mod(req)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestRedirect(t *testing.T) {
	s, _ := newServer(t)
	w := do(s.Handler(), "GET", "/claude", nil, nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://claude.ai" {
		t.Fatalf("got %d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestNotFoundSuggests(t *testing.T) {
	s, _ := newServer(t)
	w := do(s.Handler(), "GET", "/roadmp", nil, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "go/roadmap") || !strings.Contains(body, "Create go/roadmp") {
		t.Errorf("missing suggestion or create form:\n%s", body)
	}
}

func TestSearchFallsBackToFuzzy(t *testing.T) {
	s, _ := newServer(t)
	w := do(s.Handler(), "GET", "/?product%20roadmap", nil, nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://docs.example.com/roadmap" {
		t.Fatalf("lucky search: got %d %s", w.Code, w.Header().Get("Location"))
	}
	w = do(s.Handler(), "GET", "/_/search?q=road", nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "go/roadmap") {
		t.Errorf("search page: %d", w.Code)
	}
}

func TestHome(t *testing.T) {
	s, _ := newServer(t)
	w := do(s.Handler(), "GET", "/", nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "2 links") {
		t.Errorf("home: %d", w.Code)
	}
}

func TestCreateAndDelete(t *testing.T) {
	s, store := newServer(t)
	form := url.Values{"key": {"new"}, "url": {"https://new.example.com"}, "tags": {"a, b"}}
	w := do(s.Handler(), "POST", "/_/links", strings.NewReader(form.Encode()), func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://go")
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if l, ok := store.Get("new"); !ok || len(l.Tags) != 2 {
		t.Fatalf("not stored: %+v", l)
	}
	w = do(s.Handler(), "DELETE", "/_/api/links/new", nil, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestGuard(t *testing.T) {
	s, _ := newServer(t)
	h := s.Handler()
	if w := do(h, "GET", "/claude", nil, func(r *http.Request) { r.RemoteAddr = "192.168.1.20:4000" }); w.Code != http.StatusForbidden {
		t.Errorf("remote client: %d", w.Code)
	}
	if w := do(h, "GET", "/claude", nil, func(r *http.Request) { r.Host = "evil.example.com" }); w.Code != http.StatusMisdirectedRequest {
		t.Errorf("rebinding host: %d", w.Code)
	}
	form := url.Values{"key": {"x"}, "url": {"https://x.com"}}
	w := do(h, "POST", "/_/links", strings.NewReader(form.Encode()), func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://evil.example.com")
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-site POST: %d", w.Code)
	}
	w = do(h, "POST", "/_/api/links", strings.NewReader(`{"key":"x","url":"https://x.com"}`), func(r *http.Request) {
		r.Header.Set("Content-Type", "text/plain")
	})
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("non-JSON API write: %d", w.Code)
	}
}

func TestAllowTailscale(t *testing.T) {
	s, _ := newServer(t)
	s.cfg.Allow = []string{"tailscale"}
	s.cfg.Hostnames = append(s.cfg.Hostnames, "*.ts.net")
	h := s.Handler()
	phone := func(addr, host string) int {
		return do(h, "GET", "/claude", nil, func(r *http.Request) { r.RemoteAddr = addr; r.Host = host }).Code
	}
	if c := phone("100.101.102.103:5000", "go"); c != http.StatusFound {
		t.Errorf("tailnet IPv4: %d", c)
	}
	if c := phone("[fd7a:115c:a1e0::1]:5000", "go.tail1234.ts.net"); c != http.StatusFound {
		t.Errorf("tailnet IPv6 + MagicDNS name: %d", c)
	}
	if c := phone("192.168.1.20:5000", "go"); c != http.StatusForbidden {
		t.Errorf("LAN device must stay blocked: %d", c)
	}
	if c := phone("100.101.102.103:5000", "evil.example.com"); c != http.StatusMisdirectedRequest {
		t.Errorf("unknown host: %d", c)
	}
	if c := phone("100.101.102.103:5000", "ts.net.evil.com"); c != http.StatusMisdirectedRequest {
		t.Errorf("suffix trick: %d", c)
	}
}

func TestClientFilterParse(t *testing.T) {
	if _, err := newClientFilter([]string{"10.0.0.5", "192.168.0.0/16", "::1", "tailscale"}); err != nil {
		t.Fatal(err)
	}
	if _, err := newClientFilter([]string{"not-an-ip"}); err == nil {
		t.Error("bad entry should fail")
	}
}
