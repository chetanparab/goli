// Package platform holds everything that differs between macOS, Linux and
// Windows: the hosts file, opening URLs, starting goli at login, keeping the
// machine awake, and where Tailscale lives.
package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// HostsLines are the entries that make the name "go" mean this machine.
var HostsLines = []string{"127.0.0.1\tgo", "::1\tgo"}

// HostsHas reports whether hosts content already maps line's name to its address.
func HostsHas(hosts, line string) bool {
	want := strings.Fields(line)
	for _, l := range strings.Split(hosts, "\n") {
		if i := strings.Index(l, "#"); i >= 0 {
			l = l[:i]
		}
		f := strings.Fields(l)
		if len(f) < 2 || f[0] != want[0] {
			continue
		}
		for _, name := range f[1:] {
			if strings.EqualFold(name, want[1]) {
				return true
			}
		}
	}
	return false
}

// StateDir is where goli keeps its log and pid file when running in the background.
func StateDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "goli")
	}
	return os.TempDir()
}
