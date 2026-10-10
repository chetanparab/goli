package server

import (
	"fmt"
	"net"
	"strings"
)

// tailnetRanges are the address ranges Tailscale assigns to devices.
var tailnetRanges = []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"}

// clientFilter decides which remote addresses may use the server.
// Loopback is always allowed.
type clientFilter []*net.IPNet

func newClientFilter(allow []string) (clientFilter, error) {
	var f clientFilter
	for _, a := range allow {
		a = strings.TrimSpace(a)
		cidrs := []string{a}
		if strings.EqualFold(a, "tailscale") {
			cidrs = tailnetRanges
		} else if !strings.Contains(a, "/") {
			if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
				cidrs = []string{a + "/32"}
			} else {
				cidrs = []string{a + "/128"}
			}
		}
		for _, c := range cidrs {
			_, n, err := net.ParseCIDR(c)
			if err != nil {
				return nil, fmt.Errorf("allow: %q is not an IP, CIDR or \"tailscale\"", a)
			}
			f = append(f, n)
		}
	}
	return f, nil
}

func (f clientFilter) allowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, n := range f {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// hostMatcher checks Host headers against configured names; "*.suffix"
// entries match any subdomain of suffix.
type hostMatcher struct {
	exact    map[string]bool
	suffixes []string
}

func newHostMatcher(names []string) hostMatcher {
	m := hostMatcher{exact: map[string]bool{}}
	for _, h := range names {
		h = strings.ToLower(strings.TrimSpace(h))
		if strings.HasPrefix(h, "*.") {
			m.suffixes = append(m.suffixes, h[1:]) // keep the leading dot
		} else if h != "" {
			m.exact[h] = true
		}
	}
	return m
}

func (m hostMatcher) match(host string) bool {
	host = hostOnly(host)
	if m.exact[host] {
		return true
	}
	for _, s := range m.suffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}
