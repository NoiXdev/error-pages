package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/NoiXdev/error-pages/internal/config"
	"github.com/NoiXdev/error-pages/internal/pages"
	"github.com/NoiXdev/error-pages/internal/texts"
	"github.com/NoiXdev/error-pages/web"
)

func defaultConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func newRenderer(t *testing.T, cfg config.Config, version string) *Renderer {
	t.Helper()
	cat, err := texts.Load(web.FS, cfg.Theme, "", cfg.Languages)
	if err != nil {
		t.Fatalf("texts.Load: %v", err)
	}
	r, err := New(cfg, cat, web.FS, version)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func page(t *testing.T, key string) pages.Page {
	t.Helper()
	p, ok := pages.Lookup(key)
	if !ok {
		t.Fatalf("unknown page %q", key)
	}
	return p
}

func renderHTML(t *testing.T, r *Renderer, req Request) string {
	t.Helper()
	var buf bytes.Buffer
	if err := r.HTML(&buf, req); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	return buf.String()
}

var scriptTag = regexp.MustCompile(`<script[^>]*>`)

func TestHTMLGlassMaintenanceDE(t *testing.T) {
	cfg := defaultConfig(t)
	r := newRenderer(t, cfg, "1.2.3")
	out := renderHTML(t, r, Request{Page: page(t, "maintenance"), Lang: "de"})

	for _, want := range []string{
		`<html lang="de"`,
		"Diese Website ist vorübergehend nicht erreichbar",
		"Vielen Dank für Ihr Verständnis.",
		"503",
		`data-t="pages.maintenance.title"`,
		`id="ep-texts"`,
		`/static/1.2.3/themes/glass/style.css`,
		`/static/1.2.3/css/base.css`,
		`/static/1.2.3/js/i18n.js`,
		`data-action="back"`,
		`data-action="reload"`,
		`href="?lang=en"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	for _, tag := range scriptTag.FindAllString(out, -1) {
		if strings.Contains(tag, "src=") {
			continue
		}
		if tag != `<script type="application/json" id="ep-texts">` {
			t.Errorf("inline script tag %q", tag)
		}
	}
	for _, bad := range []string{`style="`, "javascript:", "<style", "{{", "<no value>"} {
		if strings.Contains(out, bad) {
			t.Errorf("output contains %q", bad)
		}
	}
}

func TestTextsJSON(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Links = []config.Link{{
		ID:     "imprint",
		URLs:   map[string]string{"de": "/impressum", "en": "/imprint", "pl": "/imprint", "fr": "/imprint"},
		Labels: map[string]string{"de": "Impressum", "en": "Imprint"},
	}}
	r := newRenderer(t, cfg, "1.2.3")
	out := renderHTML(t, r, Request{Page: page(t, "404"), Lang: "en"})

	start := strings.Index(out, `<script type="application/json" id="ep-texts">`)
	if start < 0 {
		t.Fatal("no ep-texts script")
	}
	rest := out[start+len(`<script type="application/json" id="ep-texts">`):]
	raw := rest[:strings.Index(rest, "</script>")]
	var got map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("ep-texts is not JSON: %v\n%s", err, raw)
	}
	if len(got) != 4 {
		t.Errorf("languages = %d, want 4", len(got))
	}
	de := got["de"]
	for key, want := range map[string]string{
		"pages.404.title": "Seite nicht gefunden",
		"ui.back":         "Zurück",
		"link.imprint":    "Impressum",
		"href.imprint":    "/impressum",
	} {
		if de[key] == "" || de[key] != want {
			t.Errorf("de[%q] = %q, want %q", key, de[key], want)
		}
	}
	if got["pl"]["link.imprint"] != "Imprint" {
		t.Errorf("pl label fallback = %q, want Imprint", got["pl"]["link.imprint"])
	}
	for key := range de {
		if strings.HasPrefix(key, "pages.") && !strings.HasPrefix(key, "pages.404.") {
			t.Errorf("unexpected key %q in texts JSON", key)
		}
	}
	if !strings.Contains(out, `data-link="imprint"`) || !strings.Contains(out, `href="/imprint"`) {
		t.Error("menu link missing")
	}
}

// textsFor extracts the {lang: {key: text}} object from rendered HTML.
func textsFor(t *testing.T, out string) map[string]map[string]string {
	t.Helper()
	const open = `<script type="application/json" id="ep-texts">`
	start := strings.Index(out, open)
	if start < 0 {
		t.Fatal("no ep-texts script")
	}
	rest := out[start+len(open):]
	var got map[string]map[string]string
	if err := json.Unmarshal([]byte(rest[:strings.Index(rest, "</script>")]), &got); err != nil {
		t.Fatalf("ep-texts is not JSON: %v", err)
	}
	return got
}

func TestMissingTextsFallBackToDefaultLanguage(t *testing.T) {
	load := func(t *testing.T, cfg config.Config) texts.Catalog {
		t.Helper()
		cat, err := texts.Load(web.FS, cfg.Theme, "", cfg.Languages)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range []string{"de", "pl", "fr"} {
			delete(cat[l], "pages.maintenance.closing")
		}
		return cat
	}

	t.Run("default lacks it too", func(t *testing.T) {
		cfg := defaultConfig(t)
		cfg.DefaultLanguage = "de"
		r, err := New(cfg, load(t, cfg), web.FS, "1")
		if err != nil {
			t.Fatal(err)
		}
		out := renderHTML(t, r, Request{Page: page(t, "maintenance"), Lang: "de"})
		if !strings.Contains(out, `data-t="pages.maintenance.closing" hidden`) {
			t.Error("empty closing not rendered as hidden element")
		}
		got := textsFor(t, out)
		v, ok := got["de"]["pages.maintenance.closing"]
		if !ok || v != "" {
			t.Errorf("de closing = %q, %v; want empty and present", v, ok)
		}
		if got["en"]["pages.maintenance.closing"] != "Thank you for your patience." {
			t.Error("en closing lost")
		}
	})

	t.Run("default has it", func(t *testing.T) {
		cfg := defaultConfig(t) // default language en
		r, err := New(cfg, load(t, cfg), web.FS, "1")
		if err != nil {
			t.Fatal(err)
		}
		out := renderHTML(t, r, Request{Page: page(t, "maintenance"), Lang: "de"})
		if got := textsFor(t, out)["pl"]["pages.maintenance.closing"]; got != "Thank you for your patience." {
			t.Errorf("pl closing = %q, want the en fallback", got)
		}
		// Server and client agree: the de page shows the same fallback.
		if !strings.Contains(out, `<p class="ep-closing" data-t="pages.maintenance.closing">Thank you for your patience.</p>`) {
			t.Error("server render does not use the default-language fallback")
		}
	})

	t.Run("same key set in every language", func(t *testing.T) {
		cfg := defaultConfig(t)
		r, err := New(cfg, load(t, cfg), web.FS, "1")
		if err != nil {
			t.Fatal(err)
		}
		got := textsFor(t, renderHTML(t, r, Request{Page: page(t, "maintenance"), Lang: "en"}))
		for l, m := range got {
			if len(m) != len(got["en"]) {
				t.Errorf("%s has %d keys, en has %d", l, len(m), len(got["en"]))
			}
		}
	})
}

func TestClosingAlwaysRendered(t *testing.T) {
	out := renderHTML(t, newRenderer(t, defaultConfig(t), "1"), Request{Page: page(t, "404"), Lang: "en"})
	if !strings.Contains(out, `data-t="pages.404.closing" hidden></p>`) {
		t.Error("404 page lacks the hidden closing element")
	}
}

func TestFooterSeparatorOverride(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.CopyrightName = "noidee"
	cat, err := texts.Load(web.FS, cfg.Theme, "", cfg.Languages)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := web.FS.ReadFile("templates/layout.html.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{
		"templates/layout.html.tmpl":  {Data: layout},
		"themes/glass/page.html.tmpl": {Data: []byte(`{{define "ep-sep"}} — {{end}}{{define "page"}}{{template "ep-footer" .}}{{end}}`)},
	}
	r, err := New(cfg, cat, assets, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	out := renderHTML(t, r, Request{Page: page(t, "404"), Lang: "en"})
	if !strings.Contains(out, fmt.Sprintf("© %d noidee — v1.2.3", time.Now().Year())) {
		t.Errorf("separator override not applied:\n%s", out)
	}
}

func TestFooter(t *testing.T) {
	year := time.Now().Year()
	cases := []struct {
		name    string
		mod     func(*config.Config)
		want    []string
		notWant []string
	}{
		{"full", func(c *config.Config) { c.CopyrightName = "noidee" },
			[]string{fmt.Sprintf("© %d noidee · v1.2.3", year)}, nil},
		{"no version", func(c *config.Config) { c.CopyrightName = "noidee"; c.ShowVersion = false },
			[]string{fmt.Sprintf("© %d noidee", year)}, []string{"v1.2.3"}},
		{"no footer", func(c *config.Config) { c.CopyrightName = "noidee"; c.ShowFooter = false },
			nil, []string{"©", "v1.2.3"}},
		{"empty name", func(c *config.Config) { c.CopyrightName = "" },
			[]string{"v1.2.3"}, []string{"©"}},
		{"url", func(c *config.Config) { c.CopyrightName = "noidee"; c.CopyrightURL = "https://noidee.dev" },
			[]string{`<a href="https://noidee.dev">noidee</a> · v1.2.3`}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig(t)
			tc.mod(&cfg)
			out := renderHTML(t, newRenderer(t, cfg, "1.2.3"), Request{Page: page(t, "404"), Lang: "en"})
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q", w)
				}
			}
			for _, nw := range tc.notWant {
				if strings.Contains(out, nw) {
					t.Errorf("output contains %q", nw)
				}
			}
		})
	}
}

func TestEscaping(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.CopyrightName = "<b>x</b>"
	cfg.Links = []config.Link{{ID: "x", URL: "/x", Labels: map[string]string{"en": "<img>"}}}
	out := renderHTML(t, newRenderer(t, cfg, "1.2.3"), Request{Page: page(t, "404"), Lang: "en"})
	if !strings.Contains(out, "&lt;b&gt;x&lt;/b&gt;") {
		t.Error("copyright name not escaped")
	}
	if !strings.Contains(out, "&lt;img&gt;") {
		t.Error("link label not escaped")
	}
	if strings.Contains(out, "<b>x</b>") || strings.Contains(out, "<img>") {
		t.Error("raw operator markup in output")
	}
}

func TestSingleLanguageHidesSwitch(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Languages = []string{"de"}
	cfg.DefaultLanguage = "de"
	out := renderHTML(t, newRenderer(t, cfg, "1.2.3"), Request{Page: page(t, "404"), Lang: "de"})
	if strings.Contains(out, "ep-langs") {
		t.Error("language switch rendered with one language")
	}
}

func TestUnknownLanguageFallsBack(t *testing.T) {
	cfg := defaultConfig(t)
	out := renderHTML(t, newRenderer(t, cfg, "1.2.3"), Request{Page: page(t, "404"), Lang: "xx"})
	if !strings.Contains(out, `<html lang="en"`) || !strings.Contains(out, "Page not found") {
		t.Error("unknown language did not fall back to the default")
	}
}

func TestUntil(t *testing.T) {
	cfg := defaultConfig(t)
	r := newRenderer(t, cfg, "1.2.3")
	until := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)
	out := renderHTML(t, r, Request{Page: page(t, "maintenance"), Lang: "en", Until: until})
	if !strings.Contains(out, `data-until="2026-11-10T18:00:00Z"`) {
		t.Error("data-until missing")
	}
	if !strings.Contains(out, "2026-11-10 18:00 UTC") {
		t.Error("server-side until text missing")
	}

	out = renderHTML(t, r, Request{Page: page(t, "maintenance"), Lang: "en"})
	if strings.Contains(out, "data-until") {
		t.Error("data-until rendered without Until")
	}
}

func TestJSONAndPlain(t *testing.T) {
	cfg := defaultConfig(t)
	r := newRenderer(t, cfg, "1.2.3")

	var buf bytes.Buffer
	if err := r.JSON(&buf, Request{Page: page(t, "404"), Lang: "en"}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var got struct {
		Code        int    `json:"code"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Code != 404 || got.Title != "Page not found" || got.Description == "" {
		t.Errorf("JSON = %+v", got)
	}
	if !strings.HasPrefix(buf.String(), `{"code":404,"title":`) {
		t.Errorf("JSON field order: %s", buf.String())
	}

	buf.Reset()
	if err := r.Plain(&buf, Request{Page: page(t, "404"), Lang: "en"}); err != nil {
		t.Fatalf("Plain: %v", err)
	}
	if buf.String() != "404 Page not found\n" {
		t.Errorf("Plain = %q", buf.String())
	}

	p, ok := pages.FromCode("499")
	if !ok {
		t.Fatal("FromCode(499) failed")
	}
	buf.Reset()
	if err := r.Plain(&buf, Request{Page: p, Lang: "en"}); err != nil {
		t.Fatalf("Plain: %v", err)
	}
	if buf.String() != "499 Client error\n" {
		t.Errorf("Plain 499 = %q", buf.String())
	}
	out := renderHTML(t, r, Request{Page: p, Lang: "en"})
	if !strings.Contains(out, "499") || !strings.Contains(out, "Client error") || !strings.Contains(out, `data-t="pages.4xx.title"`) {
		t.Error("499 HTML lacks code or 4xx title")
	}
}

func TestStaticPrefix(t *testing.T) {
	if got := StaticPrefix("1.0.0-rc.1"); got != "/static/1.0.0-rc.1/" {
		t.Errorf("StaticPrefix = %q", got)
	}
}

func TestNewFailsOnBrokenTemplate(t *testing.T) {
	cfg := defaultConfig(t)
	cat, err := texts.Load(web.FS, cfg.Theme, "", cfg.Languages)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := web.FS.ReadFile("templates/layout.html.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	glass, err := web.FS.ReadFile("themes/glass/page.html.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	base := fstest.MapFS{
		"templates/layout.html.tmpl":   {Data: layout},
		"themes/glass/page.html.tmpl":  {Data: glass},
		"themes/broken/page.html.tmpl": {Data: []byte(`{{define "page"}}{{if}}{{end}}`)},
	}
	if _, err := New(cfg, cat, base, "1"); err == nil {
		t.Error("New accepted a broken theme template")
	}

	delete(base, "themes/broken/page.html.tmpl")
	cfg.Theme = "terminal"
	if _, err := New(cfg, cat, base, "1"); err == nil {
		t.Error("New accepted a missing theme")
	}
}

func TestHTMLWritesNothingOnError(t *testing.T) {
	cfg := defaultConfig(t)
	cat, err := texts.Load(web.FS, cfg.Theme, "", cfg.Languages)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := web.FS.ReadFile("templates/layout.html.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{
		"templates/layout.html.tmpl":  {Data: layout},
		"themes/glass/page.html.tmpl": {Data: []byte(`{{define "page"}}<h1>{{.Title}}</h1>{{template "missing" .}}{{end}}`)},
	}
	r, err := New(cfg, cat, assets, "1")
	if err != nil {
		t.Skipf("template rejected at parse time: %v", err)
	}
	var buf bytes.Buffer
	if err := r.HTML(&buf, Request{Page: page(t, "404"), Lang: "en"}); err == nil {
		t.Fatal("HTML did not fail")
	}
	if buf.Len() != 0 {
		t.Errorf("partial output written: %q", buf.String())
	}
}
