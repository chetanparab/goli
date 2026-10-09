package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/chetanparab/goli/internal/links"
)

// DefaultAPIModel is used by the anthropic provider when no model is configured.
const DefaultAPIModel = "claude-opus-5-5"

// API ranks links with the Claude API.
type API struct {
	client anthropic.Client
	model  string
}

// NewAPI returns an API ranker. Credentials come from ANTHROPIC_API_KEY or an
// `ant auth login` profile, resolved by the SDK.
func NewAPI(model string) *API {
	if model == "" {
		model = DefaultAPIModel
	}
	return &API{client: anthropic.NewClient(option.WithMaxRetries(1)), model: model}
}

// Rank returns Claude's picks for query, filtered to keys that exist.
func (r *API) Rank(ctx context.Context, query string, all []*links.Link) ([]Pick, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cat, known := catalog(all)
	resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(r.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
			{Text: "Catalog:\n" + cat, CacheControl: anthropic.NewCacheControlEphemeralParam()},
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
		return nil, errors.New("claude declined the query")
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
	return keepKnown(out.Matches, known), nil
}
