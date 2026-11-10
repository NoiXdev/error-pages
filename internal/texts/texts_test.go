package texts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/NoiXdev/error-pages/internal/pages"
	"github.com/NoiXdev/error-pages/web"
)

var allLangs = []string{"de", "en", "pl", "fr"}

func loadBase(t *testing.T) Catalog {
	t.Helper()
	c, err := Load(web.FS, "glass", "", allLangs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func TestBaseLocalesComplete(t *testing.T) {
	c := loadBase(t)
	for _, lang := range []string{"de", "pl", "fr"} {
		for key := range c["en"] {
			if c[lang][key] == "" {
				t.Errorf("%s: key %q missing or empty", lang, key)
			}
		}
	}
	keys := append(pages.Known(), "4xx", "5xx")
	for _, lang := range allLangs {
		for _, k := range keys {
			title, desc, _ := c.Page(lang, k)
			if title == "" || title == "pages."+k+".title" || desc == "" || desc == "pages."+k+".description" {
				t.Errorf("%s: page %q lacks title or description", lang, k)
			}
		}
	}
}

func TestMaintenanceText(t *testing.T) {
	de := loadBase(t)["de"]
	checks := map[string]string{
		"pages.maintenance.title":   "Diese Website ist vorübergehend nicht erreichbar",
		"pages.maintenance.closing": "Vielen Dank für Ihr Verständnis.",
		"pages.503.title":           "Dienst vorübergehend nicht verfügbar",
	}
	for k, want := range checks {
		if got := de.T(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMergeOrder(t *testing.T) {
	assets := fstest.MapFS{
		"locales/en.yaml":              {Data: []byte("pages:\n  \"404\":\n    title: A\n    description: base\n")},
		"themes/glass/locales/en.yaml": {Data: []byte("pages:\n  \"404\":\n    title: B\n")},
	}
	op := t.TempDir()
	writeFile(t, filepath.Join(op, "locales", "en.yaml"), "pages:\n  \"404\":\n    title: X\n")
	writeFile(t, filepath.Join(op, "themes", "glass", "locales", "en.yaml"), "pages:\n  \"404\":\n    title: C\n")

	c, err := Load(assets, "glass", op, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c["en"].T("pages.404.title"); got != "C" {
		t.Errorf("title = %q, want C", got)
	}
	if got := c["en"].T("pages.404.description"); got != "base" {
		t.Errorf("description = %q, want base", got)
	}

	c, err = Load(assets, "glass", "", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c["en"].T("pages.404.title"); got != "B" {
		t.Errorf("without operator dir title = %q, want B", got)
	}
}

func TestOperatorBadYAML(t *testing.T) {
	op := t.TempDir()
	bad := filepath.Join(op, "locales", "en.yaml")
	writeFile(t, bad, "pages: [unclosed\n  - : :\n")
	_, err := Load(fstest.MapFS{"locales/en.yaml": {Data: []byte("ui:\n  back: Back\n")}}, "glass", op, []string{"en"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), bad) {
		t.Errorf("error %q does not name %q", err, bad)
	}
}

func TestOperatorUnknownKeyIgnored(t *testing.T) {
	op := t.TempDir()
	writeFile(t, filepath.Join(op, "locales", "en.yaml"), "totally:\n  unknown: value\n")
	c, err := Load(fstest.MapFS{"locales/en.yaml": {Data: []byte("ui:\n  back: Back\n")}}, "glass", op, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c["en"].T("ui.back"); got != "Back" {
		t.Errorf("ui.back = %q", got)
	}
}

func TestTFallback(t *testing.T) {
	if got := (Locale{}).T("x.y"); got != "x.y" {
		t.Errorf("got %q", got)
	}
}
