// Package config loads and validates the service configuration from
// defaults, an optional YAML file and environment variables (in that order).
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NoiXdev/error-pages/internal/pages"
)

// Themes lists the available theme names.
var Themes = []string{"glass", "terminal", "classic", "status", "clean"}

var allowedLanguages = []string{"de", "en", "pl", "fr"}

// Link is one menu link in the page footer or menu.
type Link struct {
	ID     string            `yaml:"id" json:"id"`
	URL    string            `yaml:"url" json:"url"`
	URLs   map[string]string `yaml:"urls" json:"urls"` // per language, wins over URL
	Labels map[string]string `yaml:"labels" json:"labels"`
}

// Label returns the label for lang, else for fallback, else the link ID.
func (l Link) Label(lang, fallback string) string {
	if v := l.Labels[lang]; v != "" {
		return v
	}
	if v := l.Labels[fallback]; v != "" {
		return v
	}
	return l.ID
}

// Href returns the URL for lang, else the language-independent URL.
func (l Link) Href(lang string) string {
	if v := l.URLs[lang]; v != "" {
		return v
	}
	return l.URL
}

// Config is the validated service configuration.
type Config struct {
	Theme, DefaultPage, DefaultLanguage string
	Languages                           []string

	SendSameHTTPCode, ShowDetails, ShowFooter, ShowVersion bool

	CopyrightName, CopyrightURL string
	Links                       []Link

	Maintenance      bool
	MaintenanceFile  string
	MaintenanceUntil time.Time // zero if unset

	TextsDir, Listen string
}

// fileConfig mirrors the YAML file; pointers distinguish "unset" from zero.
type fileConfig struct {
	Theme            *string  `yaml:"theme"`
	DefaultPage      *string  `yaml:"default_page"`
	Languages        []string `yaml:"languages"`
	DefaultLanguage  *string  `yaml:"default_language"`
	SendSameHTTPCode *bool    `yaml:"send_same_http_code"`
	ShowDetails      *bool    `yaml:"show_details"`
	ShowFooter       *bool    `yaml:"show_footer"`
	ShowVersion      *bool    `yaml:"show_version"`
	CopyrightName    *string  `yaml:"copyright_name"`
	CopyrightURL     *string  `yaml:"copyright_url"`
	Links            []Link   `yaml:"links"`
	Maintenance      *bool    `yaml:"maintenance"`
	MaintenanceFile  *string  `yaml:"maintenance_file"`
	MaintenanceUntil *string  `yaml:"maintenance_until"`
	TextsDir         *string  `yaml:"texts_dir"`
	Listen           *string  `yaml:"listen"`
}

func defaults() Config {
	return Config{
		Theme:            "glass",
		DefaultPage:      "404",
		DefaultLanguage:  "en",
		Languages:        []string{"de", "en", "pl", "fr"},
		SendSameHTTPCode: true,
		ShowDetails:      true,
		ShowFooter:       true,
		ShowVersion:      true,
		MaintenanceFile:  "/data/maintenance",
		Listen:           ":8080",
	}
}

func setIf[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

// Load builds the configuration. getenv has the shape of os.LookupEnv; a
// variable that is set but empty counts as unset.
func Load(getenv func(string) (string, bool)) (Config, error) {
	get := func(key string) (string, bool) {
		v, ok := getenv(key)
		return v, ok && v != ""
	}

	c := defaults()
	var until string

	if path, ok := get("CONFIG_FILE"); ok {
		raw, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("config: CONFIG_FILE: %w", err)
		}
		var f fileConfig
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
			return Config{}, fmt.Errorf("config: CONFIG_FILE %s: %w", path, err)
		}
		setIf(&c.Theme, f.Theme)
		setIf(&c.DefaultPage, f.DefaultPage)
		if f.Languages != nil {
			c.Languages = f.Languages
		}
		setIf(&c.DefaultLanguage, f.DefaultLanguage)
		setIf(&c.SendSameHTTPCode, f.SendSameHTTPCode)
		setIf(&c.ShowDetails, f.ShowDetails)
		setIf(&c.ShowFooter, f.ShowFooter)
		setIf(&c.ShowVersion, f.ShowVersion)
		setIf(&c.CopyrightName, f.CopyrightName)
		setIf(&c.CopyrightURL, f.CopyrightURL)
		if f.Links != nil {
			c.Links = f.Links
		}
		setIf(&c.Maintenance, f.Maintenance)
		setIf(&c.MaintenanceFile, f.MaintenanceFile)
		if f.MaintenanceUntil != nil {
			until = *f.MaintenanceUntil
		}
		setIf(&c.TextsDir, f.TextsDir)
		setIf(&c.Listen, f.Listen)
	}

	strs := []struct {
		key string
		dst *string
	}{
		{"THEME", &c.Theme},
		{"DEFAULT_PAGE", &c.DefaultPage},
		{"DEFAULT_LANGUAGE", &c.DefaultLanguage},
		{"COPYRIGHT_NAME", &c.CopyrightName},
		{"COPYRIGHT_URL", &c.CopyrightURL},
		{"MAINTENANCE_FILE", &c.MaintenanceFile},
		{"MAINTENANCE_UNTIL", &until},
		{"TEXTS_DIR", &c.TextsDir},
		{"LISTEN", &c.Listen},
	}
	for _, s := range strs {
		if v, ok := get(s.key); ok {
			*s.dst = v
		}
	}

	bools := []struct {
		key string
		dst *bool
	}{
		{"SEND_SAME_HTTP_CODE", &c.SendSameHTTPCode},
		{"SHOW_DETAILS", &c.ShowDetails},
		{"SHOW_FOOTER", &c.ShowFooter},
		{"SHOW_VERSION", &c.ShowVersion},
		{"MAINTENANCE", &c.Maintenance},
	}
	for _, b := range bools {
		v, ok := get(b.key)
		if !ok {
			continue
		}
		parsed, err := parseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: %s: %w", b.key, err)
		}
		*b.dst = parsed
	}

	if v, ok := get("LANGUAGES"); ok {
		c.Languages = strings.Split(v, ",")
	}
	if v, ok := get("LINKS"); ok {
		var links []Link
		if err := json.Unmarshal([]byte(v), &links); err != nil {
			return Config{}, fmt.Errorf("config: LINKS: malformed JSON: %w", err)
		}
		c.Links = links
	}

	if err := c.validate(until); err != nil {
		return Config{}, err
	}
	return c, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q (use true/false/1/0/yes/no)", s)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (c *Config) validate(until string) error {
	if !contains(Themes, c.Theme) {
		return fmt.Errorf("config: THEME: unknown theme %q (allowed: %s)", c.Theme, strings.Join(Themes, ", "))
	}

	langs := make([]string, 0, len(c.Languages))
	for _, l := range c.Languages {
		l = strings.ToLower(strings.TrimSpace(l))
		switch {
		case l == "":
			return fmt.Errorf("config: LANGUAGES: empty entry")
		case !contains(allowedLanguages, l):
			return fmt.Errorf("config: LANGUAGES: unknown language %q (allowed: %s)", l, strings.Join(allowedLanguages, ", "))
		case contains(langs, l):
			return fmt.Errorf("config: LANGUAGES: duplicate language %q", l)
		}
		langs = append(langs, l)
	}
	if len(langs) == 0 {
		return fmt.Errorf("config: LANGUAGES: at least one language is required")
	}
	c.Languages = langs

	c.DefaultLanguage = strings.ToLower(strings.TrimSpace(c.DefaultLanguage))
	if !contains(c.Languages, c.DefaultLanguage) {
		return fmt.Errorf("config: DEFAULT_LANGUAGE: %q is not in LANGUAGES (%s)", c.DefaultLanguage, strings.Join(c.Languages, ","))
	}

	if _, ok := pages.Lookup(c.DefaultPage); !ok {
		return fmt.Errorf("config: DEFAULT_PAGE: invalid page %q", c.DefaultPage)
	}

	seen := make(map[string]bool, len(c.Links))
	for i, l := range c.Links {
		if l.ID == "" {
			return fmt.Errorf("config: LINKS: link %d has no id", i)
		}
		if seen[l.ID] {
			return fmt.Errorf("config: LINKS: duplicate link id %q", l.ID)
		}
		seen[l.ID] = true
		if l.URL == "" && len(l.URLs) == 0 {
			return fmt.Errorf("config: LINKS: link %q: needs url or urls", l.ID)
		}
		if err := checkLinkURL(l.URL); err != nil {
			return fmt.Errorf("config: LINKS: link %q: url: %w", l.ID, err)
		}
		for lang, u := range l.URLs {
			if err := checkLinkURL(u); err != nil {
				return fmt.Errorf("config: LINKS: link %q: urls[%s]: %w", l.ID, lang, err)
			}
		}
		if l.URL == "" {
			for _, lang := range c.Languages {
				if l.URLs[lang] == "" {
					return fmt.Errorf("config: LINKS: link %q: no url for language %q", l.ID, lang)
				}
			}
		}
	}

	if c.CopyrightURL != "" {
		if err := checkLinkURL(c.CopyrightURL); err != nil {
			return fmt.Errorf("config: COPYRIGHT_URL: %w", err)
		}
	}

	if until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return fmt.Errorf("config: MAINTENANCE_UNTIL: not RFC 3339: %w", err)
		}
		c.MaintenanceUntil = t.UTC()
	} else {
		c.MaintenanceUntil = time.Time{}
	}
	return nil
}

// checkLinkURL accepts http(s) URLs and site-relative paths ("/x"). An empty
// value is allowed (the other URL field may carry the link).
func checkLinkURL(s string) error {
	if s == "" {
		return nil
	}
	// Browsers strip tab, CR and LF inside URLs, so "/\t/evil" would become
	// protocol-relative. Reject control characters and spaces up front.
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == ' ' {
			return fmt.Errorf("%q contains a control character or space", s)
		}
	}
	if strings.HasPrefix(s, "/") {
		if strings.HasPrefix(s, "//") || strings.HasPrefix(s, `/\`) {
			return fmt.Errorf("%q is protocol-relative", s)
		}
		return nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", s, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%q must be http(s) or a path starting with /", s)
	}
	return nil
}
