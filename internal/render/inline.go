package render

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

const (
	baseCSS   = "static/css/base.css"
	i18nJS    = "static/js/i18n.js"
	fontsDir  = "static/fonts"
	styleFile = "style.css"
)

// themeFonts lists the bundled font families each theme's style.css names.
// Each family is a stylesheet static/fonts/<family>.css; only the families
// listed here are embedded in a theme's pages.
var themeFonts = map[string][]string{
	"glass":    {"inter"},
	"classic":  {"inter"},
	"clean":    {"inter"},
	"status":   {"inter", "inconsolata"}, // Inconsolata is the monospace fallback
	"terminal": {"inconsolata"},
}

// fontURL matches the url("<file>.woff2") references of a font stylesheet.
var fontURL = regexp.MustCompile(`url\("([A-Za-z0-9._-]+\.woff2)"\)`)

// inline is the page's own CSS and JS. Pages fetch nothing: both are
// emitted inline and allowed by their hashes in the CSP. The blocks are
// built once per renderer and are the same for every page and request.
type inline struct {
	style, script template.HTML // complete <style> and <script> elements
	csp           string
}

// buildInline assembles the inline blocks for theme: the theme's fonts as
// data: URIs, base.css and the theme's style.css; i18n.js and the theme's
// theme.js if it has one.
func buildInline(assets fs.FS, theme string) (inline, error) {
	var css strings.Builder
	for _, fam := range themeFonts[theme] {
		face, err := fontFaces(assets, fam)
		if err != nil {
			return inline{}, err
		}
		css.WriteString(face)
	}
	for _, name := range []string{baseCSS, path.Join(themesDir, theme, styleFile)} {
		b, err := fs.ReadFile(assets, name)
		if err != nil {
			return inline{}, err
		}
		css.Write(b)
	}

	js, err := fs.ReadFile(assets, i18nJS)
	if err != nil {
		return inline{}, err
	}
	if b, err := fs.ReadFile(assets, path.Join(themesDir, theme, themeJS)); err == nil {
		js = append(append(js, '\n'), b...)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return inline{}, err
	}

	c, j := css.String(), string(js)
	if strings.Contains(strings.ToLower(c), "</style") {
		return inline{}, fmt.Errorf("theme %s: CSS contains </style", theme)
	}
	if strings.Contains(strings.ToLower(j), "</script") {
		return inline{}, fmt.Errorf("theme %s: JS contains </script", theme)
	}
	return inline{
		style:  template.HTML("<style>" + c + "</style>"),   // bundled, checked above
		script: template.HTML("<script>" + j + "</script>"), // bundled, checked above
		csp: "default-src 'none'; img-src data:; font-src data:; style-src 'sha256-" + hash(c) +
			"'; script-src 'sha256-" + hash(j) + "'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
	}, nil
}

// fontFaces reads static/fonts/<family>.css and replaces every font file
// reference with a data: URI of the bundled file.
func fontFaces(assets fs.FS, family string) (string, error) {
	src, err := fs.ReadFile(assets, path.Join(fontsDir, family+".css"))
	if err != nil {
		return "", err
	}
	var ferr error
	out := fontURL.ReplaceAllStringFunc(string(src), func(m string) string {
		file := fontURL.FindStringSubmatch(m)[1]
		b, err := fs.ReadFile(assets, path.Join(fontsDir, file))
		if err != nil {
			ferr = err
			return m
		}
		return "url(data:font/woff2;base64," + base64.StdEncoding.EncodeToString(b) + ")"
	})
	return out, ferr
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// CSP returns the Content-Security-Policy for this renderer's pages.
func (r *Renderer) CSP() string {
	return r.inline.csp
}
