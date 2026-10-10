// Package server is goli's HTTP redirector and web UI.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chetanparab/goli/internal/ai"
	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/ctxinfo"
	"github.com/chetanparab/goli/internal/links"
	"github.com/chetanparab/goli/internal/match"
)

//go:embed web/*.html web/*.png web/icon.svg web/manifest.webmanifest
var webFS embed.FS

// assets are static files served under /_/.
var assets = map[string]string{
	"icon-180.png":         "image/png",
	"icon-512.png":         "image/png",
	"icon.svg":             "image/svg+xml",
	"manifest.webmanifest": "application/manifest+json",
}

var tmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"pct": func(f float64) int { return int(f*100 + 0.5) },
}).ParseFS(webFS, "web/*.html"))

// Server serves redirects, search and link management.
type Server struct {
	cfg      *config.Config
	store    *links.Store
	ranker   ai.Ranker // nil when AI is disabled
	detector *ctxinfo.Detector
	log      *slog.Logger
}

// New builds a Server.
func New(cfg *config.Config, store *links.Store, log *slog.Logger) *Server {
	s := &Server{cfg: cfg, store: store, detector: ctxinfo.NewDetector(cfg.Networks), log: log}
	r, err := ai.New(cfg.AI)
	if err != nil {
		log.Error("AI search disabled", "err", err)
	}
	s.ranker = r
	return s
}

// Handler returns the HTTP handler with security middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /_/search", s.handleSearch)
	mux.HandleFunc("GET /_/{asset}", serveAsset)
	mux.HandleFunc("GET /_/api/links", s.apiList)
	mux.HandleFunc("POST /_/api/links", s.apiPut)
	mux.HandleFunc("DELETE /_/api/links/{key...}", s.apiDelete)
	mux.HandleFunc("POST /_/links", s.formPut)
	mux.HandleFunc("POST /_/links/delete", s.formDelete)
	mux.HandleFunc("GET /{path...}", s.handleGo)
	return s.guard(mux)
}

// guard only allows requests from this machine (or networks listed in
// allow), addressed to a known hostname (blocks DNS rebinding), and rejects
// cross-site writes.
func (s *Server) guard(next http.Handler) http.Handler {
	clients, err := newClientFilter(s.cfg.Allow)
	if err != nil {
		s.log.Error("ignoring allow setting", "err", err)
	}
	hosts := newHostMatcher(s.cfg.Hostnames)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !clients.allowed(net.ParseIP(host)) {
			http.Error(w, "goli only answers requests from this machine and allowed networks", http.StatusForbidden)
			return
		}
		if !hosts.match(r.Host) {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || !hosts.match(u.Host) {
					http.Error(w, "cross-site request blocked", http.StatusForbidden)
					return
				}
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

func (s *Server) context(r *http.Request) ctxinfo.Context {
	vars, err := ctxinfo.LoadVars(s.cfg.ContextPath())
	if err != nil {
		s.log.Warn("load context", "err", err)
		vars = map[string]string{}
	}
	for k, v := range links.CtxOverrides(r.URL.Query()) {
		vars[k] = v
	}
	return ctxinfo.Context{Vars: vars, Now: time.Now(), Networks: s.detector.Current()}
}

// handleGo resolves go/<path>. go/?words is a search; go/ is the home page.
func (s *Server) handleGo(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		if q := searchQuery(r); q != "" {
			s.search(w, r, q, true)
			return
		}
		s.render(w, http.StatusOK, "home.html", map[string]any{"Links": s.store.All(), "AI": s.ranker != nil})
		return
	}
	res, err := s.store.Resolve(path, r.URL.Query(), s.context(r))
	var missing *links.MissingArgsError
	switch {
	case err == nil:
		s.log.Info("redirect", "path", path, "to", res.URL)
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, res.URL, http.StatusFound)
	case errors.As(err, &missing):
		s.render(w, http.StatusBadRequest, "notfound.html", map[string]any{
			"Key": missing.Link.Key, "Missing": missing, "Suggestions": nil,
		})
	case errors.Is(err, links.ErrNotFound):
		key := links.NormalizeKey(path)
		s.render(w, http.StatusNotFound, "notfound.html", map[string]any{
			"Key":         key,
			"KeyValid":    links.ValidateKey(key) == nil,
			"Suggestions": match.Rank(strings.ReplaceAll(key, "/", " "), s.store.All(), 6),
		})
	default:
		s.fail(w, err)
	}
}

// searchQuery reads go/?some words. Browsers send the words as the raw
// query string, so "?q3 planning doc" arrives as RawQuery "q3%20planning%20doc".
func searchQuery(r *http.Request) string {
	raw := r.URL.RawQuery
	if raw == "" {
		return ""
	}
	if q := r.URL.Query().Get("q"); q != "" {
		return q
	}
	q, err := url.QueryUnescape(raw)
	if err != nil {
		return raw
	}
	return strings.TrimSpace(q)
}

func serveAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("asset")
	ctype, ok := assets[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := webFS.ReadFile("web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data) // #nosec G705 -- embedded file from the fixed assets allowlist
}

// handleSearch serves the search box. With go=1 (the home page and app box)
// it behaves like the address bar: an exact key or key/args opens directly,
// and a confident search result opens too.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	direct := r.URL.Query().Get("go") == "1"
	if direct && q != "" && !strings.ContainsAny(q, " \t") {
		path := strings.TrimPrefix(strings.TrimPrefix(q, "go/"), "/")
		if res, err := s.store.Resolve(path, nil, s.context(r)); err == nil {
			http.Redirect(w, r, res.URL, http.StatusFound)
			return
		}
	}
	s.search(w, r, q, direct || r.URL.Query().Get("lucky") == "1")
}

type searchResult struct {
	Link       *links.Link
	Confidence float64
	Reason     string
}

// search ranks links for q, with Claude when available and fuzzy matching
// otherwise. If lucky and the top hit is clearly right, it redirects.
func (s *Server) search(w http.ResponseWriter, r *http.Request, q string, lucky bool) {
	all := s.store.All()
	var results []searchResult
	var aiErr string
	usedAI := false
	if s.ranker != nil && q != "" {
		picks, err := s.ranker.Rank(r.Context(), q, all)
		if err != nil {
			s.log.Warn("ai search failed, using fuzzy match", "err", err)
			aiErr = "AI search unavailable — showing fuzzy matches instead. Run `goli doctor` for details."
		} else {
			usedAI = true
			for _, p := range picks {
				if l, ok := s.store.Get(p.Key); ok {
					results = append(results, searchResult{Link: l, Confidence: p.Confidence, Reason: p.Reason})
				}
			}
		}
	}
	if !usedAI {
		for _, m := range match.Rank(q, all, 10) {
			results = append(results, searchResult{Link: m.Link, Confidence: m.Score})
		}
	}
	if lucky && len(results) > 0 && confident(results) {
		res, err := s.store.Resolve(results[0].Link.Key, nil, s.context(r))
		if err == nil {
			http.Redirect(w, r, res.URL, http.StatusFound)
			return
		}
	}
	s.render(w, http.StatusOK, "search.html", map[string]any{
		"Query": q, "Results": results, "UsedAI": usedAI, "AIError": aiErr, "AI": s.ranker != nil,
	})
}

func confident(rs []searchResult) bool {
	top := rs[0].Confidence
	if top < 0.8 {
		return false
	}
	return len(rs) == 1 || top-rs[1].Confidence >= 0.15
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("render", "template", name, "err", err)
	}
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("request failed", "err", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// --- link management -------------------------------------------------------

func (s *Server) formPut(w http.ResponseWriter, r *http.Request) {
	l := &links.Link{
		Key:  r.FormValue("key"),
		URL:  strings.TrimSpace(r.FormValue("url")),
		Desc: strings.TrimSpace(r.FormValue("desc")),
		Tags: splitTags(r.FormValue("tags")),
	}
	if err := s.store.Put(l); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/?created="+url.QueryEscape(l.Key), http.StatusSeeOther)
}

func (s *Server) formDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.FormValue("key")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) apiList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.store.All())
}

func (s *Server) apiPut(w http.ResponseWriter, r *http.Request) {
	// Requiring a JSON body means a plain cross-site form can't hit this endpoint.
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	var l links.Link
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&l); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.Put(&l); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) apiDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.PathValue("key")); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		out = append(out, strings.ToLower(t))
	}
	return out
}

// ListenAndServe runs the server until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	s.log.Info("goli listening", "addr", s.cfg.Listen, "links", s.store.Path())
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}
