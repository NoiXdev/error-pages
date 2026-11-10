package pages

import (
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		key  string
		want Page
		ok   bool
	}{
		{"404", Page{"404", 404}, true},
		{"internal", Page{"internal", 403}, true},
		{"maintenance", Page{"maintenance", 503}, true},
		{"499", Page{"4xx", 499}, true},
		{"599", Page{"5xx", 599}, true},
		{"200", Page{}, false},
		{"600", Page{}, false},
		{"abc", Page{}, false},
		{"", Page{}, false},
		{"0503", Page{}, false},
	}
	for _, tt := range tests {
		got, ok := Lookup(tt.key)
		if ok != tt.ok || got != tt.want {
			t.Errorf("Lookup(%q) = %v, %v; want %v, %v", tt.key, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFromPath(t *testing.T) {
	tests := []struct {
		path string
		ok   bool
	}{
		{"/503", true},
		{"/503.html", true},
		{"/internal.html", true},
		{"/503/", false},
		{"/foo/503", false},
		{"/503.htm", false},
		{"/503.html.html", false},
		{"/", false},
		{"", false},
		{"/" + strings.Repeat("a", 4096), false},
	}
	for _, tt := range tests {
		_, ok := FromPath(tt.path)
		if ok != tt.ok {
			n := len(tt.path)
			t.Errorf("FromPath(len %d) ok = %v; want %v", n, ok, tt.ok)
		}
	}
	if p, _ := FromPath("/503.html"); p != (Page{"503", 503}) {
		t.Errorf("FromPath(/503.html) = %v", p)
	}
}

func TestFromCode(t *testing.T) {
	tests := []struct {
		header string
		ok     bool
	}{
		{"502", true},
		{"abc", false},
		{"302", false},
		{" 502", false},
		{"internal", false},
	}
	for _, tt := range tests {
		if _, ok := FromCode(tt.header); ok != tt.ok {
			t.Errorf("FromCode(%q) ok = %v; want %v", tt.header, ok, tt.ok)
		}
	}
}

func TestIsServerError(t *testing.T) {
	for key, want := range map[string]bool{"500": true, "404": false, "maintenance": false, "internal": false} {
		p, _ := Lookup(key)
		if p.IsServerError() != want {
			t.Errorf("%s.IsServerError() = %v; want %v", key, !want, want)
		}
	}
}

func TestFailing(t *testing.T) {
	groups := map[Link][]string{
		LinkHost:    {"500", "502", "599", "404", "409", "410", "412", "maintenance"},
		LinkNetwork: {"407", "429", "internal"},
		LinkBrowser: {"400", "401", "403", "405", "408", "411", "413", "416", "418", "499"},
	}
	for want, keys := range groups {
		for _, key := range keys {
			p, ok := Lookup(key)
			if !ok {
				t.Fatalf("Lookup(%q) not ok", key)
			}
			if got := p.Failing(); got != want {
				t.Errorf("%s.Failing() = %q; want %q", key, got, want)
			}
		}
	}
}

func TestKnownOrder(t *testing.T) {
	k := Known()
	if len(k) != 22 || k[0] != "400" || k[19] != "505" || k[20] != "internal" || k[21] != "maintenance" {
		t.Errorf("Known() = %v", k)
	}
	for _, key := range k {
		if _, ok := Lookup(key); !ok {
			t.Errorf("Lookup(%q) not ok", key)
		}
	}
}
