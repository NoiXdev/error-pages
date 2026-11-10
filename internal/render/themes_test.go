package render

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/NoiXdev/error-pages/web"
)

func TestTerminalRenders(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Theme = "terminal"
	cfg.CopyrightName = "noidee"
	r := newRenderer(t, cfg, "1.2.3")
	out := renderHTML(t, r, Request{Page: page(t, "404"), Lang: "fr"})

	for _, want := range []string{
		`<html lang="fr"`,
		"Page introuvable",
		".t-overlay",    // terminal style.css, inlined
		"SHUFFLE_STEPS", // terminal theme.js, inlined
		`data-action="back"`,
		`data-action="reload"`,
		`data-t="pages.404.title"`,
		fmt.Sprintf("© %d noidee — v1.2.3", time.Now().Year()),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(out, " · v1.2.3") {
		t.Error("terminal footer uses the default separator")
	}
	if strings.Contains(out, "style=") {
		t.Error("output contains an inline style attribute")
	}

	css, err := fs.ReadFile(web.FS, "themes/terminal/style.css")
	if err != nil {
		t.Fatalf("read terminal style.css: %v", err)
	}
	if !strings.Contains(string(css), "Inconsolata") {
		t.Error("terminal style.css does not use Inconsolata")
	}
}

func TestClassicDigits(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Theme = "classic"
	r := newRenderer(t, cfg, "1.2.3")
	out := renderHTML(t, r, Request{Page: page(t, "503"), Lang: "en"})

	for _, want := range []string{
		`<span>5</span><span>0</span><span>3</span>`,
		"body.ep-theme-classic", // classic style.css, inlined
		`data-action="back"`,
		`data-action="reload"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	sharedJSOnly(t, out)
	if strings.Contains(out, "style=") {
		t.Error("output contains an inline style attribute")
	}
}

func TestCleanRenders(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Theme = "clean"
	r := newRenderer(t, cfg, "1.2.3")
	out := renderHTML(t, r, Request{Page: page(t, "418"), Lang: "pl"})

	for _, want := range []string{
		`<html lang="pl"`,
		"Jestem czajniczkiem",
		".n-wrap", // clean style.css, inlined
		`data-t="pages.418.title"`,
		`data-action="reload"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	sharedJSOnly(t, out)
	if strings.Contains(out, "style=") {
		t.Error("output contains an inline style attribute")
	}
}

// node returns the opening tag of the chain node for link, or "".
func node(out, link string) string {
	m := regexp.MustCompile(`<li class="[^"]*" data-node="` + link + `">`).FindString(out)
	return m
}

func TestStatusFailingLink(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Theme = "status"
	r := newRenderer(t, cfg, "1.2.3")
	for key, bad := range map[string]string{"502": "host", "429": "network", "400": "browser"} {
		out := renderHTML(t, r, Request{Page: page(t, key), Lang: "en"})
		for _, link := range []string{"browser", "network", "host"} {
			tag := node(out, link)
			if tag == "" {
				t.Fatalf("%s: no node %q", key, link)
			}
			isBad := strings.Contains(tag, `class="s-node bad"`)
			if isBad != (link == bad) {
				t.Errorf("%s: node %s bad=%v, want %v", key, link, isBad, link == bad)
			}
		}
		if n := strings.Count(out, `data-t="chain.failed"`); n != 1 {
			t.Errorf("%s: %d failed nodes, want 1", key, n)
		}
		if n := strings.Count(out, `data-t="chain.working"`); n != 2 {
			t.Errorf("%s: %d working nodes, want 2", key, n)
		}
		if !strings.Contains(out, `data-t="advice.`+bad+`"`) {
			t.Errorf("%s: advice for %s missing", key, bad)
		}
		if strings.Contains(out, "style=") {
			t.Errorf("%s: output contains an inline style attribute", key)
		}
	}
	out := renderHTML(t, r, Request{Page: page(t, "502"), Lang: "en"})
	for _, want := range []string{
		`<span data-t="ui.error">Error</span> 502</b> · <span data-t="pages.502.title">Bad gateway</span>`,
		".s-wrap", // status style.css, inlined
		`data-action="back"`,
		`data-action="reload"`,
		`<svg`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

func TestStatusDetails(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Theme = "status"
	r := newRenderer(t, cfg, "1.2.3")
	at := time.Date(2026, 11, 10, 12, 3, 4, 0, time.UTC)
	out := renderHTML(t, r, Request{Page: page(t, "502"), Lang: "en",
		Details: &Details{Time: at, Host: "<script>x</script>", RequestID: "abc"}})
	if !strings.Contains(out, `<p class="s-meta">2026-11-10 12:03:04 UTC · &lt;script&gt;x&lt;/script&gt; · abc</p>`) {
		t.Errorf("meta line missing or wrong:\n%s", out)
	}
	if strings.Contains(out, "<script>x") {
		t.Error("host not escaped")
	}
	if n := strings.Count(out, "&lt;script&gt;x&lt;/script&gt;"); n != 2 {
		t.Errorf("host shown %d times, want 2 (meta and host node)", n)
	}

	out = renderHTML(t, r, Request{Page: page(t, "502"), Lang: "en"})
	if strings.Contains(markup(out), "s-meta") {
		t.Error("meta line without details")
	}
}

func TestStatusAdvice(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Theme = "status"
	r := newRenderer(t, cfg, "1.2.3")
	out := renderHTML(t, r, Request{Page: page(t, "maintenance"), Lang: "de"})
	if !strings.Contains(out, `<p data-t="advice.host">Bitte versuchen Sie es in ein paar Minuten erneut.</p>`) {
		t.Error("de advice.host missing")
	}
}
