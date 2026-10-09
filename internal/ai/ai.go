// Package ai ranks links for a natural-language query, e.g. go/?the doc with our q3 plans.
//
// Two engines are available:
//
//	claude-code  runs the local `claude` CLI, using its existing login (a Claude subscription)
//	anthropic    calls the Claude API (ANTHROPIC_API_KEY or an `ant auth login` profile)
package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/links"
)

// Pick is one link the model chose for the query.
type Pick struct {
	Key        string  `json:"key"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// Ranker ranks links for a query.
type Ranker interface {
	Rank(ctx context.Context, query string, all []*links.Link) ([]Pick, error)
}

// Provider names accepted in config.yaml.
const (
	ProviderClaudeCode = "claude-code"
	ProviderAnthropic  = "anthropic"
)

// New returns the configured Ranker, or nil if AI search is disabled.
func New(cfg config.AI) (Ranker, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	switch cfg.Provider {
	case "", ProviderClaudeCode:
		return NewClaudeCode(cfg.ClaudeBin, cfg.Model), nil
	case ProviderAnthropic:
		return NewAPI(cfg.Model), nil
	default:
		return nil, fmt.Errorf("unknown ai.provider %q (use %q or %q)", cfg.Provider, ProviderClaudeCode, ProviderAnthropic)
	}
}

const systemPrompt = `You are the search engine for a personal go-links service. Each line of the catalog is: key | description | tags | url.
Given the user's query, return the links most likely to be what they want to open, best first, at most 5. Only return keys that appear in the catalog. Use confidence near 1 only when one link is clearly the answer. Return an empty list if nothing fits.`

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"matches": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":        map[string]any{"type": "string"},
					"confidence": map[string]any{"type": "number"},
					"reason":     map[string]any{"type": "string"},
				},
				"required":             []string{"key", "confidence", "reason"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"matches"},
	"additionalProperties": false,
}

// catalog renders links one per line and returns the set of valid keys.
func catalog(all []*links.Link) (string, map[string]bool) {
	known := make(map[string]bool, len(all))
	var b strings.Builder
	for _, l := range all {
		known[l.Key] = true
		fmt.Fprintf(&b, "%s | %s | %s | %s\n", l.Key, l.Desc, strings.Join(l.Tags, ","), l.URL)
	}
	return b.String(), known
}

// keepKnown drops picks whose key isn't in the catalog.
func keepKnown(picks []Pick, known map[string]bool) []Pick {
	out := picks[:0]
	for _, p := range picks {
		p.Key = links.NormalizeKey(p.Key)
		if known[p.Key] {
			out = append(out, p)
		}
	}
	return out
}
