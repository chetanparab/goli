// Package ctxinfo decides which variant of a context-aware link applies.
//
// A variant's `when` map is matched against the current Context:
//
//	hours:   "09-18"       local hour range, end exclusive; wraps midnight ("22-06")
//	days:    "mon-fri"     day range or list ("sat,sun")
//	network: "home"        a network named in config.yaml (gateway / resolves)
//	<other>: "staging"     a context variable set with `goli ctx set env=staging`,
//	                       or per request with ?ctx.env=staging. "a|b" matches either.
package ctxinfo

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Context is everything a `when` condition can look at.
type Context struct {
	Vars     map[string]string
	Now      time.Time
	Networks []string // names of networks currently detected
}

// Matches reports whether every condition in when holds.
func (c Context) Matches(when map[string]string) bool {
	for k, want := range when {
		if !c.match(k, want) {
			return false
		}
	}
	return true
}

func (c Context) match(key, want string) bool {
	switch key {
	case "hours":
		return inHours(c.Now.Hour(), want)
	case "days":
		return inDays(c.Now.Weekday(), want)
	case "network":
		for _, alt := range strings.Split(want, "|") {
			for _, n := range c.Networks {
				if strings.EqualFold(strings.TrimSpace(alt), n) {
					return true
				}
			}
		}
		return false
	default:
		got := c.Vars[key]
		for _, alt := range strings.Split(want, "|") {
			if strings.EqualFold(strings.TrimSpace(alt), got) {
				return true
			}
		}
		return false
	}
}

func inHours(h int, spec string) bool {
	from, to, ok := strings.Cut(spec, "-")
	if !ok {
		n, err := strconv.Atoi(strings.TrimSpace(spec))
		return err == nil && n == h
	}
	a, err1 := strconv.Atoi(strings.TrimSpace(from))
	b, err2 := strconv.Atoi(strings.TrimSpace(to))
	if err1 != nil || err2 != nil {
		return false
	}
	if a <= b {
		return h >= a && h < b
	}
	return h >= a || h < b // wraps midnight
}

var dayIndex = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

func inDays(d time.Weekday, spec string) bool {
	for _, part := range strings.Split(strings.ToLower(spec), ",") {
		part = strings.TrimSpace(part)
		if from, to, ok := strings.Cut(part, "-"); ok {
			a, okA := dayIndex[from]
			b, okB := dayIndex[to]
			if !okA || !okB {
				continue
			}
			i := int(d)
			if (a <= b && i >= a && i <= b) || (a > b && (i >= a || i <= b)) {
				return true
			}
		} else if i, ok := dayIndex[part]; ok && i == int(d) {
			return true
		}
	}
	return false
}

// LoadVars reads persisted context variables.
func LoadVars(path string) (map[string]string, error) {
	vars := map[string]string{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return vars, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &vars); err != nil {
		return nil, err
	}
	return vars, nil
}

// SaveVars persists context variables.
func SaveVars(path string, vars map[string]string) error {
	data, err := yaml.Marshal(vars)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
