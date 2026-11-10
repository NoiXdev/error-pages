package render

import (
	"html"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/NoiXdev/error-pages/internal/config"
	"github.com/NoiXdev/error-pages/internal/pages"
	"github.com/NoiXdev/error-pages/internal/texts"
	"github.com/NoiXdev/error-pages/web"
)

// TestMatrix renders every theme x page x language combination and checks
// the output for the expected title and code and for template leftovers or
// CSP violations.
func TestMatrix(t *testing.T) {
	keys := append(pages.Known(), "499", "599")
	langs := []string{"de", "en", "pl", "fr"}
	forbidden := []string{"{{", "<no value>", `style="`, "javascript:"}

	for _, theme := range config.Themes {
		cfg := defaultConfig(t)
		cfg.Theme = theme
		cfg.Languages = langs
		cat, err := texts.Load(web.FS, theme, "", langs)
		if err != nil {
			t.Fatalf("%s: texts.Load: %v", theme, err)
		}
		r, err := New(cfg, cat, web.FS, "1.2.3")
		if err != nil {
			t.Fatalf("%s: New: %v", theme, err)
		}
		for _, key := range keys {
			p := page(t, key)
			for _, lang := range langs {
				t.Run(theme+"/"+key+"/"+lang, func(t *testing.T) {
					out := renderHTML(t, r, Request{Page: p, Lang: lang})
					title, _, _ := cat.Page(lang, p.Key)
					if title == "" {
						t.Fatalf("no title text for %s/%s", lang, p.Key)
					}
					if want := html.EscapeString(title); !strings.Contains(out, want) {
						t.Errorf("output lacks title %q", want)
					}
					if code := strconv.Itoa(p.Code); !strings.Contains(out, code) {
						t.Errorf("output lacks code %s", code)
					}
					for _, bad := range forbidden {
						if strings.Contains(out, bad) {
							t.Errorf("output contains %q", bad)
						}
					}
				})
			}
		}
	}
}

// TestReducedMotion asserts that every theme honours
// prefers-reduced-motion in its stylesheet and, if present, its script.
func TestReducedMotion(t *testing.T) {
	for _, theme := range config.Themes {
		css, err := fs.ReadFile(web.FS, "themes/"+theme+"/style.css")
		if err != nil {
			t.Fatalf("%s: read style.css: %v", theme, err)
		}
		if !strings.Contains(string(css), "@media (prefers-reduced-motion: reduce)") {
			t.Errorf("%s: style.css lacks a prefers-reduced-motion: reduce block", theme)
		}

		js, err := fs.ReadFile(web.FS, "themes/"+theme+"/theme.js")
		if err != nil {
			continue // theme has no script
		}
		if !strings.Contains(string(js), "matchMedia('(prefers-reduced-motion: reduce)')") {
			t.Errorf("%s: theme.js does not check matchMedia('(prefers-reduced-motion: reduce)')", theme)
		}
	}
}
