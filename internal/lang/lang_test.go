package lang

import (
	"strings"
	"testing"
)

func TestNegotiate(t *testing.T) {
	enabled := []string{"de", "en", "pl", "fr"}
	tests := []struct {
		name     string
		query    string
		header   string
		enabled  []string
		fallback string
		want     string
	}{
		{"query wins", "fr", "de", enabled, "en", "fr"},
		{"query uppercase", "FR", "de", enabled, "en", "fr"},
		{"unknown query uses header", "xx", "de", enabled, "en", "de"},
		{"q ordering", "", "de-CH;q=0.8,en;q=0.9", enabled, "en", "en"},
		{"primary subtag", "", "pl-PL", enabled, "en", "pl"},
		{"unsupported", "", "zh-Hant", enabled, "en", "en"},
		{"wildcard ignored", "", "*", enabled, "en", "en"},
		{"invalid q counts as 1", "", "de;q=abc", enabled, "en", "de"},
		{"q=0 excluded", "", "fr;q=0,de", enabled, "en", "de"},
		{"empty", "", "", enabled, "en", "en"},
		{"ties keep header order", "", "pl,de", enabled, "en", "pl"},
		{"huge header", "", strings.Repeat("xx,", 3400), enabled, "en", "en"},
		{"only first 32 entries", "", strings.Repeat("xx,", 32) + "de", enabled, "en", "en"},
		{"entry 32 counts", "", strings.Repeat("xx,", 31) + "de", enabled, "en", "de"},
		{"fallback returned as given", "", "fr", []string{"de"}, "de", "de"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Negotiate(tt.query, tt.header, tt.enabled, tt.fallback); got != tt.want {
				t.Fatalf("Negotiate(%q, %q) = %q, want %q", tt.query, tt.header, got, tt.want)
			}
		})
	}
}
