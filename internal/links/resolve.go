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

func expand(l *Link, target string, rest []string, query url.Values) (string, error) {
	names := Placeholders(target)
	if len(names) > 0 {
		next := 0
		var missing []string
		target = placeholderRe.ReplaceAllStringFunc(target, func(m string) string {
			name := m[1 : len(m)-1]
			if name == "*" {
				next = len(rest)
				return escapeSegments(rest)
			}
			idx := next
			if n, err := strconv.Atoi(name); err == nil {
				idx = n - 1
			} else {
				next++
			}
			if idx < 0 || idx >= len(rest) {
				missing = append(missing, name)
				return ""
			}
			return url.PathEscape(rest[idx])
		})
		if len(missing) > 0 {
			return "", &MissingArgsError{Link: l, Need: missing}
		}
	} else if len(rest) > 0 {
		u, err := url.Parse(target)
		if err != nil {
			return "", err
		}
		u.Path = strings.TrimSuffix(u.Path, "/") + "/" + strings.Join(rest, "/")
		u.RawPath = ""
		target = u.String()
	}
	return withQuery(target, query)
}

func escapeSegments(segs []string) string {
	esc := make([]string, len(segs))
	for i, s := range segs {
		esc[i] = url.PathEscape(s)
	}
	return strings.Join(esc, "/")
}

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
	q := u.Query()
	for k, vs := range fwd {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()
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
