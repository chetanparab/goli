package match

import (
	"testing"

	"github.com/chetanparab/goli/internal/links"
)

var all = []*links.Link{
	{Key: "roadmap", URL: "https://docs.example.com/roadmap", Desc: "Product roadmap"},
	{Key: "claude", URL: "https://claude.ai", Desc: "Claude chat", Tags: []string{"ai"}},
	{Key: "work/jira", URL: "https://example.atlassian.net", Desc: "Issue tracker"},
	{Key: "cf", URL: "https://dash.cloudflare.com", Desc: "Cloudflare dashboard"},
}

func top(q string) string {
	r := Rank(q, all, 3)
	if len(r) == 0 {
		return ""
	}
	return r[0].Link.Key
}

func TestRank(t *testing.T) {
	tests := map[string]string{
		"roadmp":     "roadmap", // typo
		"raodmap":    "roadmap", // transposition
		"road":       "roadmap", // prefix
		"jira":       "work/jira",
		"jra":        "work/jira",
		"cloudflare": "cf", // matches URL host / description
		"issues":     "work/jira",
		"ai chat":    "claude",
	}
	for q, want := range tests {
		if got := top(q); got != want {
			t.Errorf("top(%q) = %q, want %q", q, got, want)
		}
	}
	if r := Rank("zzzzzz", all, 3); len(r) != 0 {
		t.Errorf("unrelated query matched %v", r[0].Link.Key)
	}
}
