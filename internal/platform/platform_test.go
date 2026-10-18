package platform

import (
	"strings"
	"testing"
)

func TestHostsHas(t *testing.T) {
	hosts := "127.0.0.1\tlocalhost\n# 127.0.0.1 go\n127.0.0.1   go  other\n"
	if !HostsHas(hosts, "127.0.0.1\tgo") {
		t.Error("should find go")
	}
	if HostsHas(hosts, "::1\tgo") {
		t.Error("::1 isn't mapped")
	}
	if HostsHas("# 127.0.0.1 go\n", "127.0.0.1\tgo") {
		t.Error("comments don't count")
	}
}

func TestHostsInstructions(t *testing.T) {
	got := HostsInstructions(HostsLines([]string{"go", "g"}))
	for _, want := range []string{"127.0.0.1 go", "127.0.0.1 g", "goli"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions missing %q:\n%s", want, got)
		}
	}
	if HostsPath() == "" || DefaultListen == "" || Name == "" {
		t.Error("platform constants must be set")
	}
}
