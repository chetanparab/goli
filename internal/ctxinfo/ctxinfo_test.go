package ctxinfo

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMatches(t *testing.T) {
	wed := time.Date(2026, 10, 7, 23, 0, 0, 0, time.UTC) // Wednesday 23:00
	c := Context{Vars: map[string]string{"env": "staging"}, Now: wed, Networks: []string{"home"}}
	tests := []struct {
		when map[string]string
		want bool
	}{
		{map[string]string{"env": "staging"}, true},
		{map[string]string{"env": "prod|staging"}, true},
		{map[string]string{"env": "prod"}, false},
		{map[string]string{"hours": "22-06"}, true},
		{map[string]string{"hours": "09-18"}, false},
		{map[string]string{"days": "mon-fri"}, true},
		{map[string]string{"days": "sat,sun"}, false},
		{map[string]string{"days": "fri-tue"}, false},
		{map[string]string{"network": "home"}, true},
		{map[string]string{"network": "office"}, false},
		{map[string]string{"env": "staging", "network": "office"}, false},
		{map[string]string{}, true},
	}
	for _, tt := range tests {
		if got := c.Matches(tt.when); got != tt.want {
			t.Errorf("Matches(%v) = %v, want %v", tt.when, got, tt.want)
		}
	}
}

func TestVarsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "context.yaml")
	if v, err := LoadVars(path); err != nil || len(v) != 0 {
		t.Fatalf("missing file: %v %v", v, err)
	}
	if err := SaveVars(path, map[string]string{"env": "prod"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := LoadVars(path); v["env"] != "prod" {
		t.Errorf("got %v", v)
	}
}
