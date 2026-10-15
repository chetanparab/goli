package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chetanparab/goli/internal/ai"
)

// localOnly lets a handler run only for requests from this machine: other
// devices (even with allow_write) can't see or change AI settings or keys.
func (s *Server) localOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) {
			http.Error(w, "settings can only be changed on the computer running goli", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func providerName(id string) string {
	switch id {
	case ai.ProviderClaudeCode:
		return "Claude Code"
	case ai.ProviderAnthropic:
		return "Claude API"
	}
	return "Off"
}

type notice struct {
	OK   bool
	Text string
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	var n *notice
	switch r.URL.Query().Get("done") {
	case "saved":
		n = &notice{true, "Saved. AI search now uses " + providerName(ai.Active(s.cfg.AI)) + "."}
	case "key-saved":
		n = &notice{true, "API key saved in " + s.keys.Where() + "."}
	case "key-removed":
		n = &notice{true, "API key removed."}
	}
	s.renderSettings(w, r, n)
}

func (s *Server) renderSettings(w http.ResponseWriter, r *http.Request, n *notice) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	_, keyErr := s.keys.Get(ai.APIKeyName)
	s.render(w, http.StatusOK, "settings.html", map[string]any{
		"Options":          ai.Options(ctx, s.cfg.AI, s.keys),
		"Active":           ai.Active(s.cfg.AI),
		"Model":            s.cfg.AI.Model,
		"APIModels":        ai.APIModels,
		"ClaudeCodeModels": ai.ClaudeCodeModels,
		"DefaultAPIModel":  ai.DefaultAPIModel,
		"HasSavedKey":      keyErr == nil,
		"KeyWhere":         s.keys.Where(),
		"SendURLs":         s.cfg.AI.SendURLs,
		"Notice":           n,
	})
}

func (s *Server) settingsAI(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	provider := ai.NormalizeProvider(r.FormValue("provider"))
	model := ""
	switch provider {
	case ai.ProviderOff:
	case ai.ProviderClaudeCode:
		model = pick(r.FormValue("cc_model"), ai.ClaudeCodeModels)
	case ai.ProviderAnthropic:
		model = pick(r.FormValue("api_model"), ai.APIModels)
		if model == ai.DefaultAPIModel {
			model = "" // keep following the default
		}
	default:
		http.Error(w, "unknown option", http.StatusBadRequest)
		return
	}
	if err := s.cfg.SetAI(provider, model); err != nil {
		s.log.Error("save AI settings", "err", err)
		http.Error(w, "couldn't save settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.reloadAI()
	http.Redirect(w, r, "/_/settings?done=saved", http.StatusSeeOther)
}

// pick returns v if it's one of the offered model IDs, else "".
func pick(v string, models []ai.Model) string {
	for _, m := range models {
		if m.ID == v {
			return v
		}
	}
	return ""
}

func (s *Server) settingsKey(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if r.FormValue("remove") == "1" {
		if err := s.keys.Delete(ai.APIKeyName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.reloadAI()
		http.Redirect(w, r, "/_/settings?done=key-removed", http.StatusSeeOther)
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	if !strings.HasPrefix(key, "sk-ant-") {
		s.renderSettings(w, r, &notice{false, "That doesn't look like an Anthropic API key (they start with sk-ant-)."})
		return
	}
	if err := s.keys.Set(ai.APIKeyName, key); err != nil {
		s.renderSettings(w, r, &notice{false, "Couldn't save the key: " + err.Error()})
		return
	}
	s.reloadAI()
	http.Redirect(w, r, "/_/settings?done=key-saved", http.StatusSeeOther)
}

func (s *Server) settingsTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	took, err := ai.Test(ctx, s.getRanker())
	if err != nil {
		msg, _, _ := strings.Cut(err.Error(), "\n")
		s.renderSettings(w, r, &notice{false, "Test failed: " + msg})
		return
	}
	s.renderSettings(w, r, &notice{true, fmt.Sprintf("It works: %s found the right link in %.1fs.", providerName(ai.Active(s.cfg.AI)), took.Seconds())})
}
