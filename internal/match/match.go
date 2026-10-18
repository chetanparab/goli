// Package match ranks links against a typed query without any network calls.
// It powers "did you mean" suggestions and the offline half of search.
package match

import (
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/chetanparab/goli/internal/links"
)

// Result is a scored link.
type Result struct {
	Link  *links.Link `json:"link"`
	Score float64     `json:"score"`
}

// MinScore is the cut-off below which results are dropped.
const MinScore = 0.35

// Rank returns up to n links that best match q, best first.
func Rank(q string, all []*links.Link, n int) []Result {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var out []Result
	for _, l := range all {
		if s := Score(q, l); s >= MinScore {
			out = append(out, Result{Link: l, Score: s})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Link.Key < out[j].Link.Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Score rates how well a lowercase query matches a link, from 0 to 1.
func Score(q string, l *links.Link) float64 {
	best := keyScore(q, l.Key)
	for _, a := range l.Aliases {
		best = max(best, keyScore(q, a))
	}
	return min(1, max(best, 0.85*textScore(q, l))+0.05*hostScore(q, l.URL))
}

// hostScore is the fraction of query words that appear in the URL's host,
// a small tie-breaker so "cloudflare" prefers dash.cloudflare.com.
func hostScore(q, raw string) float64 {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	words := tokens(q)
	if len(words) == 0 {
		return 0
	}
	n := 0
	for _, w := range words {
		if strings.Contains(u.Hostname(), w) {
			n++
		}
	}
	return float64(n) / float64(len(words))
}

func keyScore(q, key string) float64 {
	switch {
	case q == key:
		return 1
	case strings.HasPrefix(key, q):
		return 0.9
	case strings.Contains(key, q):
		return 0.75
	}
	s := similarity(q, key)
	// Compare against each path segment too, so "jra" finds "work/jira".
	for _, seg := range strings.Split(key, "/") {
		s = max(s, 0.95*similarity(q, seg))
	}
	if isSubsequence(q, key) {
		s = max(s, 0.6)
	}
	return s
}

// textScore is the fraction of query words found in the link's key,
// description, tags or URL host/path.
func textScore(q string, l *links.Link) float64 {
	words := tokens(q)
	if len(words) == 0 {
		return 0
	}
	hay := strings.ToLower(strings.Join([]string{l.Key, strings.Join(l.Aliases, " "), l.Desc, strings.Join(l.Tags, " "), hostAndPath(l.URL)}, " "))
	hayWords := tokens(hay)
	hit := 0.0
	for _, w := range words {
		best := 0.0
		for _, h := range hayWords {
			switch {
			case h == w:
				best = 1
			case len(w) >= 3 && strings.HasPrefix(h, w):
				best = max(best, 0.9)
			case len(w) >= 4:
				best = max(best, similarity(w, h)*0.8)
			}
		}
		hit += best
	}
	return hit / float64(len(words))
}

func hostAndPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return strings.TrimPrefix(u.Host, "www.") + " " + u.Path
}

func tokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func isSubsequence(q, s string) bool {
	qr := []rune(q)
	i := 0
	for _, r := range s {
		if i < len(qr) && qr[i] == r {
			i++
		}
	}
	return i == len(qr)
}

// similarity is 1 - normalised Damerau-Levenshtein (optimal string alignment) distance.
func similarity(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	n := max(len(ra), len(rb))
	if n == 0 {
		return 1
	}
	return 1 - float64(distance(ra, rb))/float64(n)
}

func distance(a, b []rune) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}
