package links

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

// CheckStatus classifies a link health check.
type CheckStatus string

// Possible CheckStatus values.
const (
	StatusOK      CheckStatus = "ok"
	StatusAuth    CheckStatus = "auth"    // 401/403, or GitHub's 404 for private repos: probably fine
	StatusBroken  CheckStatus = "broken"  // 404/410/5xx or unreachable
	StatusSkipped CheckStatus = "skipped" // templated or local URL
)

// CheckResult is the outcome of checking one URL.
type CheckResult struct {
	Key    string
	URL    string
	Status CheckStatus
	Detail string
}

// Check requests every concrete URL (default and variants) concurrently.
// Templated URLs and, unless includeLocal, localhost URLs are skipped.
func Check(ctx context.Context, all []*Link, includeLocal bool) []CheckResult {
	type job struct{ key, url string }
	var jobs []job
	for _, l := range all {
		jobs = append(jobs, job{l.Key, l.URL})
		for _, v := range l.Variants {
			jobs = append(jobs, job{l.Key, v.URL})
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !includeLocal {
		// Checked at connect time, so redirects to internal hosts are refused too.
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, _ := net.SplitHostPort(address)
			if ip := net.ParseIP(host); ip != nil && isInternal(ip) {
				return fmt.Errorf("refusing internal address %s (use -local)", ip)
			}
			return nil
		}
	}
	client := &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 5 * time.Second},
		Timeout:   10 * time.Second,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	results := make([]CheckResult, len(jobs))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, j := range jobs {
		results[i] = CheckResult{Key: j.key, URL: j.url}
		if len(Placeholders(j.url)) > 0 {
			results[i].Status, results[i].Detail = StatusSkipped, "templated"
			continue
		}
		u, err := url.Parse(j.url)
		if err != nil {
			results[i].Status, results[i].Detail = StatusBroken, err.Error()
			continue
		}
		if !includeLocal && isLocal(u.Hostname()) {
			results[i].Status, results[i].Detail = StatusSkipped, "local"
			continue
		}
		wg.Add(1)
		go func(r *CheckResult, host string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r.Status, r.Detail = probe(ctx, client, r.URL, host)
		}(&results[i], u.Hostname())
	}
	wg.Wait()
	return results
}

// probe tries HEAD first (no side effects on well-behaved sites) and falls
// back to GET for servers that don't support it.
func probe(ctx context.Context, c *http.Client, u, host string) (CheckStatus, string) {
	resp, err := request(ctx, c, http.MethodHead, u)
	if err == nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		resp.Body.Close()
		resp, err = request(ctx, c, http.MethodGet, u)
	}
	if err != nil {
		return StatusBroken, err.Error()
	}
	resp.Body.Close()
	code := resp.StatusCode
	switch {
	case code < 400:
		return StatusOK, resp.Status
	case code == 401 || code == 403 || code == 429 || code == 999: // 999: LinkedIn bot wall
		return StatusAuth, resp.Status
	case code == 404 && host == "github.com":
		return StatusAuth, "404 (private repo or missing)"
	default:
		return StatusBroken, resp.Status
	}
}

func request(ctx context.Context, c *http.Client, method, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "goli-linkcheck/1.0")
	return c.Do(req)
}

// isInternal reports loopback, private, link-local and unspecified addresses.
func isInternal(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast()
}

func isLocal(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return isInternal(ip)
	}
	return host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || !strings.Contains(host, ".")
}
