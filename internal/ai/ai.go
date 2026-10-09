// Package ai asks Claude which links best answer a natural-language query,
// e.g. go/?the doc with our q3 plans.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/chetanparab/goli/internal/links"
)

// Pick is one link Claude chose for the query.
type Pick struct {
	Key        string  `json:"key"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// Ranker ranks links with Claude.
type Ranker struct {
	client anthropic.Client
	model  string
}

// New returns a Ranker. Credentials come from ANTHROPIC_API_KEY or an
// `ant auth login` profile, resolved by the SDK.
func New(model string) *Ranker {
	return &Ranker{client: anthropic.NewClient(option.WithMaxRetries(1)), model: model}
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

// Rank returns Claude's picks for query, filtered to keys that exist.
func (r *Ranker) Rank(ctx context.Context, query string, all []*links.Link) ([]Pick, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	known := make(map[string]bool, len(all))
	var catalog strings.Builder
	for _, l := range all {
		known[l.Key] = true
		fmt.Fprintf(&catalog, "%s | %s | %s | %s\n", l.Key, l.Desc, strings.Join(l.Tags, ","), l.URL)
	}

	resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(r.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
			{Text: "Catalog:\n" + catalog.String(), CacheControl: anthropic.NewCacheControlEphemeralParam()},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(query)),
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
	},
		// Re-serve policy declines from a fallback model chosen by the API.
		option.WithHeaderAdd("anthropic-beta", "server-side-fallback-2026-07-01"),
		option.WithJSONSet("fallbacks", "default"),
	)
	if err != nil {
		return nil, err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("claude declined the query")
	}
	var text string
	for _, b := range resp.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			text += t.Text
		}
	}
	var out struct {
		Matches []Pick `json:"matches"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("parse claude response: %w", err)
	}
	picks := out.Matches[:0]
	for _, p := range out.Matches {
		p.Key = links.NormalizeKey(p.Key)
		if known[p.Key] {
			picks = append(picks, p)
		}
	}
	return picks, nil
}
