package ctxinfo

import (
	"context"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chetanparab/goli/internal/config"
)

// Detector finds which configured networks are active, caching the answer
// briefly so redirects stay fast.
type Detector struct {
	networks map[string]config.Network
	ttl      time.Duration

	mu      sync.Mutex
	at      time.Time
	current []string
}

// NewDetector returns a Detector for the networks defined in config.
func NewDetector(networks map[string]config.Network) *Detector {
	return &Detector{networks: networks, ttl: 30 * time.Second}
}

// Current returns the names of the networks that match right now.
func (d *Detector) Current() []string {
	if len(d.networks) == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if time.Since(d.at) < d.ttl {
		return d.current
	}
	gw := defaultGateway()
	var found []string
	for name, n := range d.networks {
		if (n.Gateway != "" && n.Gateway == gw) || (n.Resolves != "" && resolves(n.Resolves)) {
			found = append(found, name)
		}
	}
	d.current, d.at = found, time.Now()
	return found
}

func defaultGateway() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var out []byte
	var err error
	switch runtime.GOOS {
	case "darwin", "freebsd":
		out, err = exec.CommandContext(ctx, "route", "-n", "get", "default").Output()
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(out), "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && k == "gateway" {
				return strings.TrimSpace(v)
			}
		}
	case "linux":
		out, err = exec.CommandContext(ctx, "ip", "route", "show", "default").Output()
		if err != nil {
			return ""
		}
		f := strings.Fields(string(out))
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "via" {
				return f[i+1]
			}
		}
	}
	return ""
}

func resolves(host string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	return err == nil && len(addrs) > 0
}
