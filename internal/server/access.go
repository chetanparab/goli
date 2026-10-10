package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// tailnetRanges are the address ranges Tailscale assigns to devices.
var tailnetRanges = []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"}

// csp locks pages to goli's own resources and forbids framing (clickjacking).
// form-action is deliberately absent: the search form redirects to external
// destinations, which form-action would block.
const csp = "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; img-src 'self' data:; " +
	"manifest-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'"

type ctxKey int

const writeKey ctxKey = 0

// canWrite reports whether the guard granted this request write access.
func canWrite(r *http.Request) bool {
	ok, _ := r.Context().Value(writeKey).(bool)
	return ok
}

// guard enforces who may talk to the server and what they may do:
//
//   - loopback clients (this machine) get full access;
//   - clients in `allow` networks are let in only if the connection also
//     arrived on an address in those networks (e.g. this Mac's Tailscale IP,
//     not a LAN that happens to use 100.64/10), and are read-only unless
//     allow_write is set;
//   - the Host header must match `hostnames` (blocks DNS rebinding);
//   - cross-origin state-changing requests are refused (CSRF).
func (s *Server) guard(next http.Handler) http.Handler {
	clients, err := newClientFilter(s.cfg.Allow)
	if err != nil {
		s.log.Error("ignoring allow setting", "err", err)
	}
	hosts := newHostMatcher(s.cfg.Hostnames)
	cop := http.NewCrossOriginProtection()
	inner := cop.Handler(next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", csp)

		remote := addrIP(r.RemoteAddr)
		write := false
		switch {
		case remote != nil && remote.IsLoopback():
			write = true
		case clients.contains(remote) && clients.contains(localIP(r)):
			write = s.cfg.AllowWrite
		default:
			http.Error(w, "goli only answers requests from this machine and allowed networks", http.StatusForbidden)
			return
		}
		if !hosts.match(r.Host) {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		if !write && r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "read-only: set allow_write: true to edit links from other devices", http.StatusForbidden)
			return
		}
		inner.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), writeKey, write)))
	})
}

func addrIP(hostport string) net.IP {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	return net.ParseIP(host)
}

// localIP is the server-side address the connection arrived on.
func localIP(r *http.Request) net.IP {
	if a, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if tcp, ok := a.(*net.TCPAddr); ok {
			return tcp.IP
		}
		return addrIP(a.String())
	}
	return nil
}

// clientFilter is the set of networks listed in `allow`.
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

func (f clientFilter) contains(ip net.IP) bool {
	if ip == nil {
		return false
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

// hostOnly strips the port, brackets and a trailing dot from a Host value.
func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.TrimSuffix(strings.ToLower(strings.Trim(h, "[]")), ".")
}
