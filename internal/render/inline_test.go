package render

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/NoiXdev/error-pages/internal/config"
	"github.com/NoiXdev/error-pages/web"
)

var (
	styleBlock  = regexp.MustCompile(`(?s)<style>(.*?)</style>`)
	scriptBlock = regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	dataURL     = regexp.MustCompile(`url\((?:"data:[^"]*"|data:[^)]*)\)`)
)

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// inlineBlocks returns the bodies of the single <style> and the single
// executable <script> element of a rendered page.
func inlineBlocks(t *testing.T, out string) (css, js string) {
	t.Helper()
	styles := styleBlock.FindAllStringSubmatch(out, -1)
	scripts := scriptBlock.FindAllStringSubmatch(out, -1)
	if len(styles) != 1 || len(scripts) != 1 {
		t.Fatalf("%d <style> and %d <script> blocks, want 1 each", len(styles), len(scripts))
	}
	return styles[0][1], scripts[0][1]
}

func TestCSPHashesMatchInlineBlocks(t *testing.T) {
	for _, theme := range config.Themes {
		t.Run(theme, func(t *testing.T) {
			cfg := defaultConfig(t)
			cfg.Theme = theme
			r := newRenderer(t, cfg, "1.2.3")
			css, js := inlineBlocks(t, renderHTML(t, r, Request{Page: page(t, "maintenance"), Lang: "de"}))
			want := "default-src 'none'; img-src data:; font-src data:; style-src 'sha256-" + sha(css) +
				"'; script-src 'sha256-" + sha(js) + "'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
			if got := r.CSP(); got != want {
				t.Errorf("CSP = %q\nwant  %q", got, want)
			}
		})
	}
}

func TestInlineBlocksIdenticalAcrossRequests(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Theme = "status"
	cfg.CopyrightName = "noidee"
	r := newRenderer(t, cfg, "1.2.3")
	css, js := inlineBlocks(t, renderHTML(t, r, Request{Page: page(t, "404"), Lang: "en"}))
	for _, req := range []Request{
		{Page: page(t, "maintenance"), Lang: "de", Until: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Page: page(t, "502"), Lang: "pl", Details: &Details{Time: time.Now(), Host: "h", RequestID: "id"}},
		{Page: page(t, "499"), Lang: "fr"},
	} {
		c, j := inlineBlocks(t, renderHTML(t, r, req))
		if c != css || j != js {
			t.Errorf("%s/%s: inline blocks differ between requests", req.Page.Key, req.Lang)
		}
	}
}

func TestPagesAreSelfContained(t *testing.T) {
	for _, theme := range config.Themes {
		t.Run(theme, func(t *testing.T) {
			cfg := defaultConfig(t)
			cfg.Theme = theme
			r := newRenderer(t, cfg, "1.2.3")
			for _, key := range []string{"404", "maintenance"} {
				out := renderHTML(t, r, Request{Page: page(t, key), Lang: "de"})
				low := strings.ToLower(out)
				for _, bad := range []string{"/static", "<link", " src=", "@import"} {
					if strings.Contains(low, bad) {
						t.Errorf("%s: output contains %q", key, bad)
					}
				}
				css, js := inlineBlocks(t, out)
				if rest := dataURL.ReplaceAllString(css, ""); strings.Contains(rest, "url(") {
					t.Errorf("%s: CSS has a url( that is not a data: URI", key)
				}
				if strings.Contains(css, "</") || strings.Contains(js, "</") {
					t.Errorf("%s: inline block contains a closing tag", key)
				}
				if !strings.Contains(js, "ep:lang") {
					t.Errorf("%s: script lacks i18n.js", key)
				}
			}
		})
	}
}

func TestThemeFonts(t *testing.T) {
	families := map[string]string{"inter": `font-family: "Inter"`, "inconsolata": `font-family: "Inconsolata"`}
	expected := map[string]string{
		"glass": "inter", "classic": "inter", "clean": "inter",
		"status": "inter", "terminal": "inconsolata",
	}
	for _, theme := range config.Themes {
		t.Run(theme, func(t *testing.T) {
			declared, ok := themeFonts[theme]
			if !ok {
				t.Fatal("theme has no font declaration in themeFonts")
			}
			if got := strings.Join(declared, ","); got != expected[theme] {
				t.Errorf("themeFonts = %q, want %q", got, expected[theme])
			}
			style, err := fs.ReadFile(web.FS, "themes/"+theme+"/style.css")
			if err != nil {
				t.Fatal(err)
			}
			cfg := defaultConfig(t)
			cfg.Theme = theme
			css, _ := inlineBlocks(t, renderHTML(t, newRenderer(t, cfg, "1"), Request{Page: page(t, "404"), Lang: "en"}))
			for fam, face := range families {
				want := false
				for _, d := range declared {
					want = want || d == fam
				}
				// The declaration must match what the stylesheet names.
				name := strings.TrimPrefix(face, "font-family: ")
				if uses := strings.Contains(string(style), name); uses != want {
					t.Errorf("%s: style.css uses it = %v, declared = %v", fam, uses, want)
				}
				if n := strings.Count(css, face+";"); want && n != 2 || !want && n != 0 {
					t.Errorf("%s: %d @font-face rules, want latin + latin-ext = %v", fam, n, want)
				}
			}
			if got, want := strings.Count(css, "url(data:font/woff2;base64,"), 2*len(declared); got != want {
				t.Errorf("%d embedded fonts, want %d", got, want)
			}
			if !strings.Contains(css, "unicode-range") && len(declared) > 0 {
				t.Error("unicode-range descriptors lost")
			}
		})
	}
}

// TestInlineBreakoutGuard checks that buildInline rejects theme sources that
// could end the inline element early or switch the script into the
// double-escaped state ("<!--" followed by "<script").
func TestInlineBreakoutGuard(t *testing.T) {
	for name, files := range map[string]fstest.MapFS{
		"style":   {"themes/x/style.css": {Data: []byte("a{} </STYLE>")}},
		"script":  {"themes/x/style.css": {}, "themes/x/theme.js": {Data: []byte("var a = '</Script>';")}},
		"comment": {"themes/x/style.css": {}, "themes/x/theme.js": {Data: []byte("var a = '<!--';")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := buildInline(testAssets(t, files), "x"); err == nil {
				t.Error("buildInline accepted a breakout sequence")
			}
		})
	}
	if _, err := buildInline(testAssets(t, fstest.MapFS{"themes/x/style.css": {}}), "x"); err != nil {
		t.Errorf("clean theme rejected: %v", err)
	}
}

// TestStatusUsesSystemMonospace checks that the status theme's details line
// relies on the system monospace stack instead of an embedded font.
func TestStatusUsesSystemMonospace(t *testing.T) {
	style, err := fs.ReadFile(web.FS, "themes/status/style.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(style), "font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;") {
		t.Error("status style.css lacks the system monospace stack")
	}
}
