// Package bookmarks reads browser bookmark exports so `goli import` can turn
// them into links: the Netscape HTML file every browser can export, and the
// JSON file Chrome, Edge and Brave keep on disk.
package bookmarks

import (
	"encoding/json"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// Bookmark is one saved page.
type Bookmark struct {
	Title  string
	URL    string
	Folder string // "Bookmarks Bar/Work", "" at the top level
}

// Parse detects the format from the content.
func Parse(data []byte) ([]Bookmark, error) {
	trim := strings.TrimSpace(string(data))
	switch {
	case strings.HasPrefix(trim, "{"):
		return parseChrome(data)
	case strings.Contains(strings.ToUpper(trim[:min(len(trim), 400)]), "NETSCAPE-BOOKMARK") || strings.Contains(trim, "<DL>") || strings.Contains(trim, "<dl>"):
		return parseNetscape(trim), nil
	}
	return nil, errors.New("not a bookmarks file (expected a browser's HTML export or Chrome's Bookmarks JSON)")
}

var (
	anchorRe = regexp.MustCompile(`(?is)<a\s[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	tokenRe  = regexp.MustCompile(`(?is)<h3[^>]*>(.*?)</h3>|</dl>|<a\s[^>]*href="[^"]*"[^>]*>.*?</a>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
)

// parseNetscape walks the <H3> folder headings and </DL> closers to keep a
// folder path for each <A>.
func parseNetscape(s string) []Bookmark {
	var out []Bookmark
	var path []string
	for _, m := range tokenRe.FindAllStringSubmatch(s, -1) {
		tok := m[0]
		switch {
		case strings.HasPrefix(strings.ToLower(tok), "<h3"):
			path = append(path, clean(m[1]))
		case strings.EqualFold(tok, "</dl>"):
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		default:
			a := anchorRe.FindStringSubmatch(tok)
			if a == nil {
				continue
			}
			if b, ok := keep(html.UnescapeString(a[1]), clean(a[2]), strings.Join(path, "/")); ok {
				out = append(out, b)
			}
		}
	}
	return out
}

func clean(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, "")))
}

type chromeNode struct {
	Type     string       `json:"type"`
	Name     string       `json:"name"`
	URL      string       `json:"url"`
	Children []chromeNode `json:"children"`
}

func parseChrome(data []byte) ([]Bookmark, error) {
	var doc struct {
		Roots map[string]chromeNode `json:"roots"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var out []Bookmark
	var walk func(n chromeNode, path string)
	walk = func(n chromeNode, path string) {
		if n.Type == "url" {
			if b, ok := keep(n.URL, n.Name, path); ok {
				out = append(out, b)
			}
			return
		}
		for _, c := range n.Children {
			walk(c, strings.Trim(path+"/"+n.Name, "/"))
		}
	}
	for _, root := range []string{"bookmark_bar", "other", "synced"} {
		if n, ok := doc.Roots[root]; ok {
			walk(n, "")
		}
	}
	return out, nil
}

// keep drops what can't become a link: non-http URLs and empty titles.
func keep(raw, title, folder string) (Bookmark, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Bookmark{}, false
	}
	if title == "" {
		title = u.Host
	}
	return Bookmark{Title: title, URL: u.String(), Folder: folder}, true
}

var nonKey = regexp.MustCompile(`[^a-z0-9]+`)

// Key suggests a link key for a bookmark: the first three words of its
// title, or its host when the title is useless. Callers handle collisions.
func Key(b Bookmark) string {
	words := strings.Fields(nonKey.ReplaceAllString(strings.ToLower(b.Title), " "))
	var parts []string
	for _, w := range words {
		if stop[w] {
			continue
		}
		parts = append(parts, w)
		if len(parts) == 3 {
			break
		}
	}
	key := strings.Join(parts, "-")
	if len(key) > 24 {
		key = strings.TrimRight(key[:24], "-")
	}
	if key == "" {
		if u, err := url.Parse(b.URL); err == nil {
			h := strings.TrimPrefix(u.Hostname(), "www.")
			key, _, _ = strings.Cut(h, ".")
		}
	}
	return key
}

var stop = map[string]bool{"the": true, "a": true, "an": true, "and": true, "of": true, "to": true, "in": true, "on": true, "for": true, "my": true, "your": true, "home": true, "page": true, "www": true, "com": true, "http": true, "https": true}
