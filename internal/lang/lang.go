// Package lang negotiates the display language from the lang query value and
// the Accept-Language header.
package lang

import (
	"sort"
	"strconv"
	"strings"
)

const maxEntries = 32

type entry struct {
	tag string
	q   float64
}

// Negotiate returns one of enabled or fallback. A valid lang query value wins;
// otherwise the Accept-Language header is evaluated by q (ties keep header
// order), matching on the lowercase primary subtag.
func Negotiate(query, acceptLanguage string, enabled []string, fallback string) string {
	if tag := primary(query); tag != "" && contains(enabled, tag) {
		return tag
	}
	for _, e := range parse(acceptLanguage) {
		if contains(enabled, e.tag) {
			return e.tag
		}
	}
	return fallback
}

func parse(header string) []entry {
	var entries []entry
	for i, part := range strings.SplitN(header, ",", maxEntries+1) {
		if i >= maxEntries {
			break
		}
		fields := strings.Split(part, ";")
		tag := primary(fields[0])
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		for _, p := range fields[1:] {
			p = strings.TrimSpace(p)
			if len(p) > 2 && (p[0] == 'q' || p[0] == 'Q') && p[1] == '=' {
				if v, err := strconv.ParseFloat(p[2:], 64); err == nil && v >= 0 && v <= 1 {
					q = v
				}
			}
		}
		if q == 0 {
			continue
		}
		entries = append(entries, entry{tag, q})
	}
	sort.SliceStable(entries, func(a, b int) bool { return entries[a].q > entries[b].q })
	return entries
}

func primary(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		s = s[:i]
	}
	return s
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
