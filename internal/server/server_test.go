package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chetanparab/goli/internal/ai"
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

// viaTailnet makes a request look like it arrived on this machine's tailnet address.
func viaTailnet(remote, host string) func(*http.Request) {
	return func(r *http.Request) {
		r.RemoteAddr = remote
		r.Host = host
		local := &net.TCPAddr{IP: net.ParseIP("100.109.1.1"), Port: 80}
		*r = *r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, local))
	}
}

func TestAllowTailscale(t *testing.T) {
	s, _ := newServer(t)
	s.cfg.Allow = []string{"tailscale"}
	s.cfg.Hostnames = append(s.cfg.Hostnames, "*.tail1234.ts.net")
	h := s.Handler()
	get := func(mod func(*http.Request)) int { return do(h, "GET", "/claude", nil, mod).Code }

	if c := get(viaTailnet("100.101.102.103:5000", "go")); c != http.StatusFound {
		t.Errorf("tailnet IPv4: %d", c)
	}
	if c := get(viaTailnet("[fd7a:115c:a1e0::1]:5000", "go.tail1234.ts.net")); c != http.StatusFound {
		t.Errorf("tailnet IPv6 + MagicDNS name: %d", c)
	}
	if c := get(viaTailnet("192.168.1.20:5000", "go")); c != http.StatusForbidden {
		t.Errorf("LAN device must stay blocked: %d", c)
	}
	if c := get(viaTailnet("100.101.102.103:5000", "evil.example.com")); c != http.StatusMisdirectedRequest {
		t.Errorf("unknown host: %d", c)
	}
	if c := get(viaTailnet("100.101.102.103:5000", "go.other.ts.net")); c != http.StatusMisdirectedRequest {
		t.Errorf("someone else's tailnet name: %d", c)
	}
	// A 100.64/10 client on a LAN (carrier-grade NAT), not on our tailnet address.
	if c := get(func(r *http.Request) {
		r.RemoteAddr = "100.72.1.5:4000"
		local := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 80}
		*r = *r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, local))
	}); c != http.StatusForbidden {
		t.Errorf("CGNAT LAN client: %d", c)
	}
}

func TestAllowedNetworksReadOnly(t *testing.T) {
	s, store := newServer(t)
	s.cfg.Allow = []string{"tailscale"}
	body := `{"key":"mail","url":"https://phish.example.com"}`
	post := func() int {
		return do(s.Handler(), "POST", "/_/api/links", strings.NewReader(body), func(r *http.Request) {
			viaTailnet("100.72.1.5:4000", "go")(r)
			r.Header.Set("Content-Type", "application/json")
		}).Code
	}
	if c := post(); c != http.StatusForbidden {
		t.Errorf("tailnet write without allow_write: %d", c)
	}
	if _, ok := store.Get("mail"); ok {
		t.Fatal("link was written")
	}
	s.cfg.AllowWrite = true
	if c := post(); c != http.StatusOK {
		t.Errorf("tailnet write with allow_write: %d", c)
	}
}

func TestCrossSiteWritesBlocked(t *testing.T) {
	s, store := newServer(t)
	h := s.Handler()
	form := url.Values{"key": {"x"}, "url": {"https://x.example.com"}}.Encode()
	cases := map[string]func(*http.Request){
		"other localhost port": func(r *http.Request) { r.Header.Set("Origin", "http://localhost:3000") },
		"Sec-Fetch-Site":       func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"same-site sibling":    func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") },
	}
	for name, mod := range cases {
		w := do(h, "POST", "/_/links", strings.NewReader(form), func(r *http.Request) {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			mod(r)
		})
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: got %d", name, w.Code)
		}
	}
	if _, ok := store.Get("x"); ok {
		t.Error("cross-site request created a link")
	}
}

type countingRanker struct{ calls int }

func (c *countingRanker) Rank(context.Context, string, []*links.Link) ([]ai.Pick, error) {
	c.calls++
	return []ai.Pick{{Key: "claude", Confidence: 0.95}}, nil
}

func TestAIOnlyForUserSearches(t *testing.T) {
	s, _ := newServer(t)
	r := &countingRanker{}
	s.ranker = r
	h := s.Handler()
	// Another site embedding <img src="http://go/?..."> must not spend AI calls.
	do(h, "GET", "/?some%20query", nil, func(req *http.Request) {
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Sec-Fetch-Dest", "image")
	})
	if r.calls != 0 {
		t.Fatalf("cross-site request used AI (%d calls)", r.calls)
	}
	w := do(h, "GET", "/?chat%20assistant", nil, func(req *http.Request) {
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-Dest", "document")
	})
	if r.calls != 1 || w.Code != http.StatusFound {
		t.Fatalf("typed search: calls=%d code=%d", r.calls, w.Code)
	}
	do(h, "GET", "/?chat%20assistant", nil, nil)
	if r.calls != 1 {
		t.Errorf("repeat search should be cached, calls=%d", r.calls)
	}
}

func TestLimitsAndHeaders(t *testing.T) {
	s, _ := newServer(t)
	h := s.Handler()
	if w := do(h, "GET", "/"+strings.Repeat("a", 2000), nil, nil); w.Code != http.StatusRequestURITooLong {
		t.Errorf("long path: %d", w.Code)
	}
	w := do(h, "GET", "/", nil, nil)
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") ||
		w.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("missing anti-framing headers: %v", w.Header())
	}
	if strings.Contains(w.Body.String(), "onsubmit") {
		t.Error("inline handler would be blocked by CSP")
	}
}

func TestWebEditKeepsVariants(t *testing.T) {
	s, store := newServer(t)
	store.Put(&links.Link{Key: "dash", URL: "https://prod.example.com", Desc: "Dashboard",
		Variants: []links.Variant{{When: map[string]string{"env": "dev"}, URL: "http://localhost:3000"}}})
	form := url.Values{"key": {"dash"}, "url": {"https://prod2.example.com"}}.Encode()
	do(s.Handler(), "POST", "/_/links", strings.NewReader(form), func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	})
	l, _ := store.Get("dash")
	if l.URL != "https://prod2.example.com" || len(l.Variants) != 1 || l.Desc != "Dashboard" {
		t.Errorf("edit dropped fields: %+v", l)
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

func TestSearchBoxOpensKeysDirectly(t *testing.T) {
	s, _ := newServer(t)
	h := s.Handler()
	for _, q := range []string{"claude", "go/claude", "CLAUDE"} {
		w := do(h, "GET", "/_/search?go=1&q="+url.QueryEscape(q), nil, nil)
		if w.Code != http.StatusFound || w.Header().Get("Location") != "https://claude.ai" {
			t.Errorf("q=%q: %d %s", q, w.Code, w.Header().Get("Location"))
		}
	}
	// Without go=1 it's a normal results page.
	if w := do(h, "GET", "/_/search?q=claude", nil, nil); w.Code != http.StatusOK {
		t.Errorf("plain search: %d", w.Code)
	}
}

func TestAssets(t *testing.T) {
	s, _ := newServer(t)
	h := s.Handler()
	for _, a := range []string{"icon-180.png", "icon-512.png", "icon.svg", "manifest.webmanifest"} {
		if w := do(h, "GET", "/_/"+a, nil, nil); w.Code != 200 || w.Body.Len() == 0 {
			t.Errorf("%s: %d", a, w.Code)
		}
	}
	if w := do(h, "GET", "/_/secret", nil, nil); w.Code != 404 {
		t.Errorf("unknown asset: %d", w.Code)
	}
}
