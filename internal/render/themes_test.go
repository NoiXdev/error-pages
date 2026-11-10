package render

import (
	"fmt"
	"io/fs"
	"regexp"
	"slices"
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

var liTag = regexp.MustCompile(`<li\s[^>]*>`)

// node returns the opening <li> tag that carries data-node="<link>", in
// whatever attribute order, or "".
func node(out, link string) string {
	for _, tag := range liTag.FindAllString(out, -1) {
		if strings.Contains(tag, ` data-node="`+link+`"`) {
			return tag
		}
	}
	return ""
}

// hasClass reports whether an opening tag's class attribute lists name.
func hasClass(tag, name string) bool {
	m := regexp.MustCompile(`\sclass="([^"]*)"`).FindStringSubmatch(tag)
	return m != nil && slices.Contains(strings.Fields(m[1]), name)
}

func TestNodeHelperIgnoresAttributeOrder(t *testing.T) {
	for _, out := range []string{
		`<li class="s-node bad" data-node="host">`,
		`<li data-node="host" class="s-node bad">`,
		`<li data-x="1" data-node="host" class="bad s-node">`,
	} {
		if tag := node(out, "host"); tag == "" || !hasClass(tag, "bad") {
			t.Errorf("%s: node/hasClass failed", out)
		}
	}
	if node(`<li data-node="hosts">`, "host") != "" {
		t.Error("matched a different node")
	}
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
			isBad := hasClass(tag, "bad")
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
	for _, tc := range []struct{ key, lang, want string }{
		{"maintenance", "de", `<p data-t="advice.host">Bitte versuchen Sie es in ein paar Minuten erneut.</p>`},
		{"internal", "en", `<p data-t="advice.network">Check your connection or network access and reload the page in a few moments.</p>`},
		{"429", "de", `<p data-t="advice.network">Prüfen Sie Ihre Verbindung bzw. Ihren Zugang zum Netz und laden Sie die Seite in einigen Augenblicken neu.</p>`},
		{"400", "en", `<p data-t="advice.browser">Check the address and try again. If the problem persists, contact support.</p>`},
		{"499", "de", `<p data-t="advice.browser">Prüfen Sie die Adresse und versuchen Sie es erneut. Besteht das Problem weiter, wenden Sie sich an den Support.</p>`},
	} {
		if out := renderHTML(t, r, Request{Page: page(t, tc.key), Lang: tc.lang}); !strings.Contains(out, tc.want) {
			t.Errorf("%s/%s: lacks %q", tc.key, tc.lang, tc.want)
		}
	}
}

func TestClassicDigitsVariants(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Theme = "classic"
	r := newRenderer(t, cfg, "1.2.3")
	for key, want := range map[string]string{
		"499":      `aria-label="499"><span>4</span><span>9</span><span>9</span></div>`,
		"internal": `aria-label="403"><span>4</span><span>0</span><span>3</span></div>`,
	} {
		if out := renderHTML(t, r, Request{Page: page(t, key), Lang: "en"}); !strings.Contains(out, want) {
			t.Errorf("%s: lacks %q", key, want)
		}
	}
}
