package links

import (
	"context"
	"net/http"
	"net/url"
	"sync"
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
	client := &http.Client{
		Timeout: 10 * time.Second,
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

func probe(ctx context.Context, c *http.Client, u, host string) (CheckStatus, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return StatusBroken, err.Error()
	}
	req.Header.Set("User-Agent", "goli-linkcheck/1.0")
	resp, err := c.Do(req)
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

func isLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		len(host) > 10 && host[len(host)-10:] == ".localhost"
}
