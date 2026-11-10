package render

import (
	"fmt"
	"io/fs"
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
		`/static/1.2.3/themes/terminal/style.css`,
		`/static/1.2.3/themes/terminal/theme.js`,
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
