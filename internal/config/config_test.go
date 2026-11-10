package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "glass" || c.DefaultPage != "404" || c.DefaultLanguage != "en" || c.Listen != ":8080" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if !reflect.DeepEqual(c.Languages, []string{"de", "en", "pl", "fr"}) {
		t.Fatalf("languages = %v", c.Languages)
	}
	if !c.SendSameHTTPCode || !c.ShowDetails || !c.ShowFooter || !c.ShowVersion {
		t.Fatalf("flags not all true: %+v", c)
	}
	if c.MaintenanceFile != "/data/maintenance" || c.Maintenance || !c.MaintenanceUntil.IsZero() {
		t.Fatalf("maintenance defaults wrong: %+v", c)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	p := writeFile(t, "theme: terminal\ncopyright_name: A\n")
	c, err := Load(env(map[string]string{"CONFIG_FILE": p, "THEME": "clean"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "clean" || c.CopyrightName != "A" {
		t.Fatalf("theme=%q name=%q", c.Theme, c.CopyrightName)
	}
}

func TestEmptyEnvKeepsLowerLayer(t *testing.T) {
	p := writeFile(t, "theme: terminal\n")
	c, err := Load(env(map[string]string{"CONFIG_FILE": p, "THEME": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "terminal" {
		t.Fatalf("theme = %q", c.Theme)
	}
}

func TestFileFull(t *testing.T) {
	p := writeFile(t, `
languages: [en, de]
default_language: de
show_footer: false
maintenance: true
maintenance_until: "2026-11-10T18:00:00+01:00"
links:
  - id: privacy
    url: /privacy
    urls:
      de: /datenschutz
    labels:
      de: Datenschutz
`)
	c, err := Load(env(map[string]string{"CONFIG_FILE": p}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Languages, []string{"en", "de"}) || c.DefaultLanguage != "de" {
		t.Fatalf("languages: %+v", c)
	}
	if c.ShowFooter || !c.Maintenance {
		t.Fatalf("bools: %+v", c)
	}
	if want := time.Date(2026, 11, 10, 17, 0, 0, 0, time.UTC); !c.MaintenanceUntil.Equal(want) || c.MaintenanceUntil.Location() != time.UTC {
		t.Fatalf("until = %v", c.MaintenanceUntil)
	}
	if len(c.Links) != 1 || c.Links[0].Href("de") != "/datenschutz" || c.Links[0].Href("en") != "/privacy" {
		t.Fatalf("links: %+v", c.Links)
	}
}

func TestLinksFromEnv(t *testing.T) {
	c, err := Load(env(map[string]string{
		"LINKS": `[{"id":"privacy","url":"/privacy","labels":{"de":"Datenschutz","en":"Privacy"}},{"id":"bare","url":"/x"}]`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	l := c.Links[0]
	if got := l.Label("pl", "en"); got != "Privacy" {
		t.Errorf("fallback label = %q", got)
	}
	if got := l.Label("de", "en"); got != "Datenschutz" {
		t.Errorf("de label = %q", got)
	}
	if got := c.Links[1].Label("de", "en"); got != "bare" {
		t.Errorf("id label = %q", got)
	}
}

func TestValidationErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string // empty: must pass
	}{
		{"unknown theme", map[string]string{"THEME": "neon"}, "THEME"},
		{"unknown language", map[string]string{"LANGUAGES": "en,xx"}, "LANGUAGES"},
		{"duplicate language", map[string]string{"LANGUAGES": "en,de,en"}, "LANGUAGES"},
		{"empty language entry", map[string]string{"LANGUAGES": "en,,de"}, "LANGUAGES"},
		{"default language not enabled", map[string]string{"LANGUAGES": "de,fr", "DEFAULT_LANGUAGE": "en"}, "DEFAULT_LANGUAGE"},
		{"invalid default page", map[string]string{"DEFAULT_PAGE": "teapot"}, "DEFAULT_PAGE"},
		{"malformed links", map[string]string{"LINKS": "{nope"}, "LINKS"},
		{"link without id", map[string]string{"LINKS": `[{"url":"/x"}]`}, "LINKS"},
		{"duplicate link id", map[string]string{"LINKS": `[{"id":"a","url":"/x"},{"id":"a","url":"/y"}]`}, "duplicate"},
		{"javascript url", map[string]string{"LINKS": `[{"id":"a","url":"javascript:alert(1)"}]`}, "LINKS"},
		{"data url", map[string]string{"LINKS": `[{"id":"a","url":"data:text/html,x"}]`}, "LINKS"},
		{"data copyright url", map[string]string{"COPYRIGHT_URL": "data:text/html,x"}, "COPYRIGHT_URL"},
		{"javascript per-language url", map[string]string{"LINKS": `[{"id":"a","urls":{"de":"javascript:x"}}]`}, "LINKS"},
		{"protocol-relative url", map[string]string{"LINKS": `[{"id":"a","url":"//evil.org"}]`}, "LINKS"},
		{"tab slash url", map[string]string{"LINKS": `[{"id":"a","url":"/\t/evil"}]`}, "LINKS"},
		{"newline slash url", map[string]string{"LINKS": `[{"id":"a","url":"/\n/evil"}]`}, "LINKS"},
		{"cr slash url", map[string]string{"LINKS": `[{"id":"a","url":"/\r/evil"}]`}, "LINKS"},
		{"tab backslash url", map[string]string{"LINKS": `[{"id":"a","url":"/\t\\evil"}]`}, "LINKS"},
		{"space in url", map[string]string{"LINKS": `[{"id":"a","url":"/a b"}]`}, "LINKS"},
		{"javascript copyright url", map[string]string{"COPYRIGHT_URL": "javascript:alert(1)"}, "COPYRIGHT_URL"},
		{"https copyright url", map[string]string{"COPYRIGHT_URL": "https://x.org"}, ""},
		{"link without url or urls", map[string]string{"LINKS": `[{"id":"a"}]`}, "needs url or urls"},
		{"urls missing a language", map[string]string{"LANGUAGES": "de,en", "LINKS": `[{"id":"a","urls":{"de":"/d"}}]`}, `"en"`},
		{"urls cover all languages", map[string]string{"LANGUAGES": "de,en", "LINKS": `[{"id":"a","urls":{"de":"/d","en":"/e"}}]`}, ""},
		{"maintenance default page", map[string]string{"DEFAULT_PAGE": "maintenance"}, ""},
		{"https url", map[string]string{"LINKS": `[{"id":"a","url":"https://x.org/p"}]`}, ""},
		{"bad until", map[string]string{"MAINTENANCE_UNTIL": "tomorrow"}, "MAINTENANCE_UNTIL"},
		{"missing config file", map[string]string{"CONFIG_FILE": missing}, "CONFIG_FILE"},
		{"bad bool", map[string]string{"SHOW_FOOTER": "maybe"}, "SHOW_FOOTER"},
		{"normalized languages", map[string]string{"LANGUAGES": " EN , De "}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(env(tc.env))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.HasPrefix(err.Error(), "config:") || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q should start with config: and name %s", err, tc.wantErr)
			}
		})
	}
}

func TestBool(t *testing.T) {
	for in, want := range map[string]bool{"no": false, "0": false, "false": false, "yes": true, "1": true, "true": true} {
		c, err := Load(env(map[string]string{"SHOW_FOOTER": in}))
		if err != nil {
			t.Fatal(err)
		}
		if c.ShowFooter != want {
			t.Errorf("SHOW_FOOTER=%s: got %v", in, c.ShowFooter)
		}
	}
}

func TestUntilFromEnvUTC(t *testing.T) {
	c, err := Load(env(map[string]string{"MAINTENANCE_UNTIL": "2026-11-10T18:00:00+01:00"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.MaintenanceUntil.Hour() != 17 || c.MaintenanceUntil.Location() != time.UTC {
		t.Fatalf("until = %v", c.MaintenanceUntil)
	}
}

func TestUnknownYAMLKey(t *testing.T) {
	p := writeFile(t, "thme: terminal\n")
	_, err := Load(env(map[string]string{"CONFIG_FILE": p}))
	if err == nil || !strings.HasPrefix(err.Error(), "config:") {
		t.Fatalf("expected config error, got %v", err)
	}
}

func TestEmptyYAMLFile(t *testing.T) {
	p := writeFile(t, "")
	c, err := Load(env(map[string]string{"CONFIG_FILE": p}))
	if err != nil || c.Theme != "glass" {
		t.Fatalf("err=%v theme=%q", err, c.Theme)
	}
}

func TestEnvBoolOverridesFile(t *testing.T) {
	p := writeFile(t, "show_footer: false\n")
	c, err := Load(env(map[string]string{"CONFIG_FILE": p, "SHOW_FOOTER": "true"}))
	if err != nil || !c.ShowFooter {
		t.Fatalf("err=%v footer=%v", err, c.ShowFooter)
	}
}

func TestEnvLanguagesReplaceFile(t *testing.T) {
	p := writeFile(t, "languages: [de, fr, pl]\n")
	c, err := Load(env(map[string]string{"CONFIG_FILE": p, "LANGUAGES": "en,de"}))
	if err != nil || !reflect.DeepEqual(c.Languages, []string{"en", "de"}) {
		t.Fatalf("err=%v languages=%v", err, c.Languages)
	}
}
