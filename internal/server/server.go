// Package server is goli's HTTP redirector and web UI.
package server

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chetanparab/goli/internal/ai"
	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/ctxinfo"
	"github.com/chetanparab/goli/internal/links"
	"github.com/chetanparab/goli/internal/match"
)

//go:embed web/*.html web/*.png web/icon.svg web/*.js web/manifest.webmanifest web/fonts/*.woff2
var webFS embed.FS

// assets are static files served under /_/.
var assets = map[string]string{
	"app.js":               "text/javascript; charset=utf-8",
	"theme.js":             "text/javascript; charset=utf-8",
	"fonts/fraunces.woff2": "font/woff2",
	"fonts/hanken.woff2":   "font/woff2",
	"icon-180.png":         "image/png",
	"icon-512.png":         "image/png",
	"icon.svg":             "image/svg+xml",
	"manifest.webmanifest": "application/manifest+json",
}

var tmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"pct": func(f float64) int { return int(f*100 + 0.5) },
}).ParseFS(webFS, "web/*.html"))

// Limits that keep a hostile page from making the server do expensive work.
const (
	maxInputLen   = 512              // longest path or search query handled
	maxFormBytes  = 64 << 10         // form and JSON bodies
	aiTimeout     = 25 * time.Second // must stay well under WriteTimeout
	aiConcurrency = 2
	aiCacheTTL    = 10 * time.Minute
	aiCacheSize   = 64
)

// Server serves redirects, search and link management.
type Server struct {
	cfg      *config.Config
	store    *links.Store
	ranker   ai.Ranker // nil when AI is disabled
	detector *ctxinfo.Detector
	log      *slog.Logger

	aiSem   chan struct{}
	cacheMu sync.Mutex
	cache   map[string]cachedPicks
}

type cachedPicks struct {
	picks []ai.Pick
	at    time.Time
}

// New builds a Server.
func New(cfg *config.Config, store *links.Store, log *slog.Logger) *Server {
	s := &Server{
		cfg: cfg, store: store, detector: ctxinfo.NewDetector(cfg.Networks), log: log,
		aiSem: make(chan struct{}, aiConcurrency), cache: map[string]cachedPicks{},
	}
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
	mux.HandleFunc("GET /_/fonts/{font}", serveAsset)
	mux.HandleFunc("GET /_/api/links", s.apiList)
	mux.HandleFunc("POST /_/api/links", s.apiPut)
	mux.HandleFunc("DELETE /_/api/links/{key...}", s.apiDelete)
	mux.HandleFunc("POST /_/links", s.formPut)
	mux.HandleFunc("POST /_/links/delete", s.formDelete)
	mux.HandleFunc("GET /{path...}", s.handleGo)
	return s.guard(mux)
}

// ListenAndServe runs the server until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
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
	if len(path) > maxInputLen || len(r.URL.RawQuery) > maxInputLen {
		http.Error(w, "request too long", http.StatusRequestURITooLong)
		return
	}
	if path == "" {
		if q := searchQuery(r); q != "" {
			s.search(w, r, q, true)
			return
		}
		form := linkForm{}
		if key := r.URL.Query().Get("edit"); key != "" && canWrite(r) {
			if l, ok := s.store.Get(key); ok {
				form = linkForm{Key: l.Key, URL: l.URL, Desc: l.Desc, Tags: strings.Join(l.Tags, ", "), Editing: true}
			}
		}
		s.render(w, http.StatusOK, "home.html", map[string]any{
			"Links": s.store.All(), "AI": s.ranker != nil, "CanWrite": canWrite(r), "Form": form,
		})
		return
	}
	res, err := s.store.Resolve(path, r.URL.Query(), s.context(r))
	var missing *links.MissingArgsError
	switch {
	case err == nil:
		// Log the key, not the destination: URLs can carry private IDs or tokens.
		s.log.Info("redirect", "key", res.Link.Key)
		s.log.Debug("redirect target", "path", path, "to", res.URL)
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
			"Form":        linkForm{Key: key},
			"KeyValid":    links.ValidateKey(key) == nil,
			"CanWrite":    canWrite(r),
			"Suggestions": match.Rank(strings.ReplaceAll(key, "/", " "), s.store.All(), 6),
		})
	default:
		// e.g. a template argument that can't go in a host name
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// searchQuery reads go/?some words. Browsers send the words as the raw
// query string, so "?q3 planning doc" arrives as RawQuery "q3%20planning%20doc".
// A query made only of ctx.* overrides is not a search.
func searchQuery(r *http.Request) string {
	raw := r.URL.RawQuery
	if raw == "" {
		return ""
	}
	params := r.URL.Query()
	if q := params.Get("q"); q != "" {
		return q
	}
	if len(params) > 0 && len(links.CtxOverrides(params)) == len(params) {
		return ""
	}
	// Home-page UI parameters (?edit=key, ?saved=key) aren't searches.
	if params.Has("edit") || params.Has("saved") {
		return ""
	}
	q, err := url.QueryUnescape(raw)
	if err != nil {
		return raw
	}
	return strings.TrimSpace(q)
}

func serveAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("asset")
	if f := r.PathValue("font"); f != "" {
		name = "fonts/" + f
	}
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
	// Revalidate by content hash, so a goli upgrade is picked up immediately.
	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Write(data) // #nosec G705 -- embedded file from the fixed assets allowlist
}

// handleSearch serves the search box. With go=1 (the home page and app box)
// it behaves like the address bar: an exact key or key/args opens directly,
// and a confident search result opens too.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > maxInputLen {
		http.Error(w, "query too long", http.StatusRequestURITooLong)
		return
	}
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

// linkForm pre-fills the create/edit form.
type linkForm struct {
	Key, URL, Desc, Tags string
	Editing              bool
}

type searchResult struct {
	Link       *links.Link
	Confidence float64
	Reason     string
}

// search ranks links for q, with AI when available and fuzzy matching
// otherwise. If lucky and the top hit is clearly right, it redirects.
func (s *Server) search(w http.ResponseWriter, r *http.Request, q string, lucky bool) {
	all := s.store.All()
	var results []searchResult
	var aiErr string
	usedAI := false
	if s.ranker != nil && q != "" && userInitiated(r) {
		picks, err := s.rankAI(r.Context(), q, all)
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

// userInitiated reports whether a request came from the user typing in the
// address bar, using goli's own pages, or a non-browser client — as opposed to
// another website embedding go/ URLs. Only those may spend AI calls.
func userInitiated(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "none", "same-origin":
	default:
		return false
	}
	switch r.Header.Get("Sec-Fetch-Dest") {
	case "", "document":
		return true
	}
	return false
}

// rankAI calls the ranker with a concurrency limit and a short-lived cache.
func (s *Server) rankAI(ctx context.Context, q string, all []*links.Link) ([]ai.Pick, error) {
	key := strings.ToLower(q)
	s.cacheMu.Lock()
	if c, ok := s.cache[key]; ok && time.Since(c.at) < aiCacheTTL {
		s.cacheMu.Unlock()
		return c.picks, nil
	}
	s.cacheMu.Unlock()

	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	default:
		return nil, errors.New("too many AI searches in flight")
	}
	ctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	picks, err := s.ranker.Rank(ctx, q, all)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	if len(s.cache) >= aiCacheSize {
		clear(s.cache)
	}
	s.cache[key] = cachedPicks{picks: picks, at: time.Now()}
	s.cacheMu.Unlock()
	return picks, nil
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

// --- link management -------------------------------------------------------

func (s *Server) formPut(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	l := &links.Link{
		Key:  r.FormValue("key"),
		URL:  strings.TrimSpace(r.FormValue("url")),
		Desc: strings.TrimSpace(r.FormValue("desc")),
		Tags: splitTags(r.FormValue("tags")),
	}
	if r.FormValue("mode") == "edit" {
		// The edit form shows every field, so an empty one means "cleared";
		// only variants (not editable here) are carried over.
		if old, ok := s.store.Get(l.Key); ok {
			l.Variants = old.Variants
		}
	} else {
		s.mergeExisting(l)
	}
	if err := s.store.Put(l); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/?saved="+url.QueryEscape(l.Key), http.StatusSeeOther)
}

// mergeExisting keeps fields an update didn't supply (variants are only
// editable in links.yaml, so a web edit must not drop them).
func (s *Server) mergeExisting(l *links.Link) {
	old, ok := s.store.Get(l.Key)
	if !ok {
		return
	}
	if l.Desc == "" {
		l.Desc = old.Desc
	}
	if len(l.Tags) == 0 {
		l.Tags = old.Tags
	}
	if len(l.Variants) == 0 {
		l.Variants = old.Variants
	}
}

func (s *Server) formDelete(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxFormBytes)).Decode(&l); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mergeExisting(&l)
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
