package links

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/chetanparab/goli/internal/ctxinfo"
)

// ErrNotFound means no key matched the requested path.
var ErrNotFound = errors.New("link not found")

// MissingArgsError is returned when a templated link is used without enough path segments.
type MissingArgsError struct {
	Link *Link
	Need []string
}

func (e *MissingArgsError) Error() string {
	return fmt.Sprintf("go/%s needs: %s", e.Link.Key, strings.Join(e.Need, ", "))
}

// Resolution is the result of resolving a go/ path.
type Resolution struct {
	Link *Link
	URL  string
}

// CtxQueryPrefix marks query parameters that set context instead of being forwarded.
const CtxQueryPrefix = "ctx."

// Resolve maps a path like "jira/ABC-123" to a destination URL.
//
// The longest key that prefixes the path wins. Remaining segments fill
// placeholders ({1}, {2}, {*} or named {id}) or, if the URL has none, are
// appended to it. Query parameters are forwarded, except ctx.* ones.
func (s *Store) Resolve(path string, query url.Values, c ctxinfo.Context) (*Resolution, error) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return nil, ErrNotFound
	}
	for i := len(parts); i > 0; i-- {
		l, ok := s.Get(strings.Join(parts[:i], "/"))
		if !ok {
			continue
		}
		target := pickURL(l, c)
		u, err := expand(l, target, parts[i:], query)
		if err != nil {
			return nil, err
		}
		return &Resolution{Link: l, URL: u}, nil
	}
	return nil, ErrNotFound
}

func splitPath(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// pickURL returns the first variant whose conditions match, else the default URL.
func pickURL(l *Link, c ctxinfo.Context) string {
	for _, v := range l.Variants {
		if c.Matches(v.When) {
			return v.URL
		}
	}
	return l.URL
}

var placeholderRe = regexp.MustCompile(`\{([a-zA-Z0-9_*]+)\}`)

// Placeholders lists the placeholders in a URL template, in order.
func Placeholders(u string) []string {
	var out []string
	for _, m := range placeholderRe.FindAllStringSubmatch(u, -1) {
		out = append(out, m[1])
	}
	return out
}

// URL parts a placeholder can sit in; each needs different escaping.
const (
	partHost = iota
	partPath
	partQuery
)

var hostLabelRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// partAt reports which part of a URL template the byte offset i falls in.
func partAt(tmpl string, i int) int {
	hostStart := strings.Index(tmpl, "://") + 3
	hostEnd := len(tmpl)
	if j := strings.IndexAny(tmpl[hostStart:], "/?#"); j >= 0 {
		hostEnd = hostStart + j
	}
	if i < hostEnd {
		return partHost
	}
	if q := strings.IndexAny(tmpl, "?#"); q >= 0 && i > q {
		return partQuery
	}
	return partPath
}

func escapeFor(part int, s string) (string, error) {
	switch part {
	case partHost:
		if !hostLabelRe.MatchString(s) {
			return "", fmt.Errorf("%q can't be used in a host name", s)
		}
		return s, nil
	case partQuery:
		return url.QueryEscape(s), nil
	default:
		return url.PathEscape(s), nil
	}
}

func expand(l *Link, target string, rest []string, query url.Values) (string, error) {
	if len(Placeholders(target)) > 0 {
		var b strings.Builder
		next, last := 0, 0
		var missing []string
		for _, m := range placeholderRe.FindAllStringSubmatchIndex(target, -1) {
			b.WriteString(target[last:m[0]])
			last = m[1]
			name := target[m[2]:m[3]]
			part := partAt(target, m[0])
			var val string
			switch name {
			case "*":
				next = len(rest)
				switch part {
				case partQuery:
					val = url.QueryEscape(strings.Join(rest, "/"))
				case partHost:
					return "", fmt.Errorf("go/%s: {*} can't be used in a host name", l.Key)
				default:
					val = escapeSegments(rest)
				}
			default:
				idx := next
				if n, err := strconv.Atoi(name); err == nil {
					idx = n - 1
				} else {
					next++
				}
				if idx < 0 || idx >= len(rest) {
					missing = append(missing, name)
					continue
				}
				v, err := escapeFor(part, rest[idx])
				if err != nil {
					return "", fmt.Errorf("go/%s: %w", l.Key, err)
				}
				val = v
			}
			b.WriteString(val)
		}
		b.WriteString(target[last:])
		if len(missing) > 0 {
			return "", &MissingArgsError{Link: l, Need: missing}
		}
		target = b.String()
	} else if len(rest) > 0 {
		u, err := url.Parse(target)
		if err != nil {
			return "", err
		}
		extra := strings.Join(rest, "/")
		if strings.HasPrefix(u.Fragment, "/") {
			// Hash-routed single-page app (https://app/#/home): extend the route.
			u.Fragment = strings.TrimSuffix(u.Fragment, "/") + "/" + extra
		} else {
			u.Path = strings.TrimSuffix(u.Path, "/") + "/" + extra
			u.RawPath = ""
		}
		target = u.String()
	}
	out, err := withQuery(target, query)
	if err != nil {
		return "", err
	}
	// Whatever was substituted, the result must still be a plain web URL.
	u, err := url.Parse(out)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("go/%s: expanded to an invalid URL", l.Key)
	}
	return out, nil
}

func escapeSegments(segs []string) string {
	esc := make([]string, len(segs))
	for i, s := range segs {
		esc[i] = url.PathEscape(s)
	}
	return strings.Join(esc, "/")
}

// withQuery appends forwarded query parameters (except ctx.*) without
// re-encoding or reordering the destination's own query string.
func withQuery(target string, query url.Values) (string, error) {
	fwd := url.Values{}
	for k, vs := range query {
		if !strings.HasPrefix(k, CtxQueryPrefix) {
			fwd[k] = vs
		}
	}
	if len(fwd) == 0 {
		return target, nil
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if u.RawQuery == "" {
		u.RawQuery = fwd.Encode()
	} else {
		u.RawQuery += "&" + fwd.Encode()
	}
	return u.String(), nil
}

// CtxOverrides extracts ?ctx.key=value parameters.
func CtxOverrides(query url.Values) map[string]string {
	out := map[string]string{}
	for k, vs := range query {
		if strings.HasPrefix(k, CtxQueryPrefix) && len(vs) > 0 {
			out[strings.TrimPrefix(k, CtxQueryPrefix)] = vs[0]
		}
	}
	return out
}
