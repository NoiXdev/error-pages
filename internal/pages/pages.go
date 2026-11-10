// Package pages is the registry of error pages the service can render.
package pages

import (
	"strconv"
	"strings"
)

// Link names the part of the connection chain that is failing.
type Link string

// Link values, from the visitor outwards.
const (
	LinkBrowser Link = "browser"
	LinkNetwork Link = "network"
	LinkHost    Link = "host"
)

const (
	keyInternal    = "internal"
	keyMaintenance = "maintenance"
	keyClient      = "4xx"
	keyServer      = "5xx"
)

// Page identifies one renderable page.
type Page struct {
	Key  string // text key: "404", "4xx", "5xx", "internal", "maintenance"
	Code int    // status code shown and sent
}

var knownCodes = []int{
	400, 401, 403, 404, 405, 407, 408, 409, 410, 411, 412, 413, 416, 418, 429,
	500, 502, 503, 504, 505,
}

// Known returns the keys of all dedicated pages: the 20 codes, then
// "internal" and "maintenance".
func Known() []string {
	keys := make([]string, 0, len(knownCodes)+2)
	for _, c := range knownCodes {
		keys = append(keys, strconv.Itoa(c))
	}
	return append(keys, keyInternal, keyMaintenance)
}

// Lookup resolves a page key or a numeric 4xx/5xx code. Codes without a
// dedicated page map to the class keys "4xx" or "5xx".
func Lookup(key string) (Page, bool) {
	switch key {
	case keyInternal:
		return Page{Key: keyInternal, Code: 403}, true
	case keyMaintenance:
		return Page{Key: keyMaintenance, Code: 503}, true
	}
	code, ok := parseCode(key)
	if !ok {
		return Page{}, false
	}
	for _, c := range knownCodes {
		if c == code {
			return Page{Key: key, Code: code}, true
		}
	}
	if code < 500 {
		return Page{Key: keyClient, Code: code}, true
	}
	return Page{Key: keyServer, Code: code}, true
}

// FromPath resolves a request path such as "/503", "/503.html" or
// "/internal.html". It accepts exactly one path segment.
func FromPath(path string) (Page, bool) {
	key, ok := strings.CutPrefix(path, "/")
	if !ok || len(key) > 64 || strings.Contains(key, "/") {
		return Page{}, false
	}
	key = strings.TrimSuffix(key, ".html")
	return Lookup(key)
}

// FromCode resolves an X-Code header value. Only numeric 400-599 qualify.
func FromCode(header string) (Page, bool) {
	if _, ok := parseCode(header); !ok {
		return Page{}, false
	}
	return Lookup(header)
}

// IsServerError reports whether the page is a genuine 5xx failure
// (maintenance is planned, not an error).
func (p Page) IsServerError() bool {
	return p.Code >= 500 && p.Key != keyMaintenance
}

// Failing returns the link in the chain that the page blames.
func (p Page) Failing() Link {
	switch {
	case p.Key == keyInternal, p.Code == 407, p.Code == 429:
		return LinkNetwork
	case p.Code >= 500, p.Key == keyMaintenance:
		return LinkHost
	}
	switch p.Code {
	case 404, 409, 410, 412:
		return LinkHost
	}
	return LinkBrowser
}

// parseCode strictly parses a three-digit code without leading zero in 400-599.
func parseCode(s string) (int, bool) {
	if len(s) != 3 || s[0] < '4' || s[0] > '5' {
		return 0, false
	}
	n := 0
	for i := 0; i < 3; i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}
