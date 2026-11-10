// Package render turns a page request into HTML, JSON or plain-text bodies.
package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/NoiXdev/error-pages/internal/config"
	"github.com/NoiXdev/error-pages/internal/pages"
	"github.com/NoiXdev/error-pages/internal/texts"
)

const (
	layoutFile = "templates/layout.html.tmpl"
	themesDir  = "themes"
	pageFile   = "page.html.tmpl"
	themeJS    = "theme.js"
	untilFmt   = "2006-01-02 15:04 UTC"
)

// sharedPrefixes are the catalog key prefixes embedded for the client-side
// language switch, in addition to the current page's keys.
var sharedPrefixes = []string{"ui.", "chain.", "advice."}

// Details are the request facts shown by themes that display them.
type Details struct {
	Time            time.Time
	Host, RequestID string
}

// Request describes one page to render.
type Request struct {
	Page    pages.Page
	Lang    string
	Until   time.Time // maintenance end, zero if none
	Details *Details  // nil unless cfg.ShowDetails (status theme uses it)
}

// Renderer renders pages with the configured theme. It is safe for
// concurrent use.
type Renderer struct {
	cfg        config.Config
	cat        texts.Catalog
	tmpl       *template.Template
	version    string
	static     string
	hasThemeJS bool
	textsJSON  map[string]template.JS // by page key
}

// Lang is one entry of the language switch.
type Lang struct {
	Code, Label string
	Href        template.URL
	Current     bool
}

// Link is one menu link in the current language.
type Link struct {
	ID, Label, Href string
}

// Footer is the footer line; Show is false when nothing is to be shown.
type Footer struct {
	Year      int
	Name, URL string
	Version   string // empty when hidden
	Show      bool
}

// View is the model passed to the templates.
type View struct {
	Lang    string
	Langs   []Lang
	Code    int
	PageKey string

	T                           func(key string) string
	Title, Description, Closing string

	Links     []Link
	Footer    Footer
	Until     string // RFC 3339, or empty
	UntilText string // ui.expected_back with the time in UTC, or empty
	Details   *Details

	Static     string
	Theme      string
	HasThemeJS bool
	TextsJSON  template.JS
}

// StaticPrefix is the URL prefix of the versioned static assets.
func StaticPrefix(version string) string {
	return "/static/" + version + "/"
}

// New parses the shared layout and every theme template found in assets.
// A broken template in any theme, or a missing template for cfg.Theme, is
// an error.
func New(cfg config.Config, cat texts.Catalog, assets fs.FS, version string) (*Renderer, error) {
	layout, err := fs.ReadFile(assets, layoutFile)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	base, err := template.New("layout").Parse(string(layout))
	if err != nil {
		return nil, fmt.Errorf("render: parse %s: %w", layoutFile, err)
	}

	entries, err := fs.ReadDir(assets, themesDir)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	var selected *template.Template
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := path.Join(themesDir, e.Name(), pageFile)
		src, err := fs.ReadFile(assets, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("render: %w", err)
		}
		t, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("render: %w", err)
		}
		if _, err := t.Parse(string(src)); err != nil {
			return nil, fmt.Errorf("render: parse %s: %w", name, err)
		}
		if t.Lookup("page") == nil {
			return nil, fmt.Errorf("render: %s does not define \"page\"", name)
		}
		if e.Name() == cfg.Theme {
			selected = t
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("render: no template for theme %q", cfg.Theme)
	}

	_, err = fs.Stat(assets, path.Join(themesDir, cfg.Theme, themeJS))
	r := &Renderer{
		cfg:        cfg,
		cat:        cat,
		tmpl:       selected,
		version:    version,
		static:     StaticPrefix(version),
		hasThemeJS: err == nil,
		textsJSON:  map[string]template.JS{},
	}

	keys := append(pages.Known(), "4xx", "5xx")
	for _, k := range keys {
		js, err := r.buildTextsJSON(k)
		if err != nil {
			return nil, fmt.Errorf("render: texts for %s: %w", k, err)
		}
		r.textsJSON[k] = js
	}
	return r, nil
}

// buildTextsJSON collects {lang: {key: text}} for the client-side switch.
func (r *Renderer) buildTextsJSON(pageKey string) (template.JS, error) {
	pagePrefix := "pages." + pageKey + "."
	out := make(map[string]map[string]string, len(r.cfg.Languages))
	for _, lang := range r.cfg.Languages {
		m := map[string]string{}
		for k, v := range r.cat[lang] {
			if strings.HasPrefix(k, pagePrefix) || hasAnyPrefix(k, sharedPrefixes) {
				m[k] = v
			}
		}
		for _, l := range r.cfg.Links {
			m["link."+l.ID] = l.Label(lang, r.cfg.DefaultLanguage)
			m["href."+l.ID] = l.Href(lang)
		}
		out[lang] = m
	}
	b, err := json.Marshal(out) // escapes <, > and & for the script context
	if err != nil {
		return "", err
	}
	return template.JS(b), nil // json.Marshal output is script-safe
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// lang returns req.Lang if it is enabled, else the default language.
func (r *Renderer) lang(req Request) string {
	for _, l := range r.cfg.Languages {
		if l == req.Lang {
			return l
		}
	}
	return r.cfg.DefaultLanguage
}

func (r *Renderer) view(req Request) View {
	lang := r.lang(req)
	loc := r.cat[lang]
	title, desc, closing := r.cat.Page(lang, req.Page.Key)

	v := View{
		Lang:        lang,
		Code:        req.Page.Code,
		PageKey:     req.Page.Key,
		T:           loc.T,
		Title:       title,
		Description: desc,
		Closing:     closing,
		Details:     req.Details,
		Static:      r.static,
		Theme:       r.cfg.Theme,
		HasThemeJS:  r.hasThemeJS,
		TextsJSON:   r.textsJSON[req.Page.Key],
	}
	if _, ok := r.textsJSON[req.Page.Key]; !ok {
		v.TextsJSON, _ = r.buildTextsJSON(req.Page.Key)
	}
	if len(r.cfg.Languages) > 1 {
		for _, l := range r.cfg.Languages {
			v.Langs = append(v.Langs, Lang{
				Code:    l,
				Label:   strings.ToUpper(l),
				Href:    template.URL("?lang=" + l), // l is a validated language code
				Current: l == lang,
			})
		}
	}
	for _, l := range r.cfg.Links {
		v.Links = append(v.Links, Link{ID: l.ID, Label: l.Label(lang, r.cfg.DefaultLanguage), Href: l.Href(lang)})
	}

	f := Footer{Year: time.Now().Year(), Name: r.cfg.CopyrightName, URL: r.cfg.CopyrightURL}
	if r.cfg.ShowVersion {
		f.Version = r.version
	}
	f.Show = r.cfg.ShowFooter && (f.Name != "" || f.Version != "")
	v.Footer = f

	if !req.Until.IsZero() {
		u := req.Until.UTC()
		v.Until = u.Format(time.RFC3339)
		v.UntilText = strings.ReplaceAll(loc.T("ui.expected_back"), "{time}", u.Format(untilFmt))
	}
	return v
}

// HTML renders the themed page. Output is buffered, so nothing is written
// when the template fails.
func (r *Renderer) HTML(w io.Writer, req Request) error {
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, "layout", r.view(req)); err != nil {
		return fmt.Errorf("render: %w", err)
	}
	_, err := buf.WriteTo(w)
	return err
}

// JSON renders {"code":…,"title":…,"description":…}.
func (r *Renderer) JSON(w io.Writer, req Request) error {
	title, desc, _ := r.cat.Page(r.lang(req), req.Page.Key)
	return json.NewEncoder(w).Encode(struct {
		Code        int    `json:"code"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}{req.Page.Code, title, desc})
}

// Plain renders "<code> <title>\n".
func (r *Renderer) Plain(w io.Writer, req Request) error {
	title, _, _ := r.cat.Page(r.lang(req), req.Page.Key)
	_, err := io.WriteString(w, strconv.Itoa(req.Page.Code)+" "+title+"\n")
	return err
}
