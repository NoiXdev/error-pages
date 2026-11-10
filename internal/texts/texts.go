// Package texts loads and merges the localized text catalog.
package texts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Locale maps flattened dotted keys ("pages.404.title") to texts.
type Locale map[string]string

// Catalog holds one Locale per language.
type Catalog map[string]Locale

// T returns the text for key, or the key itself when it is missing.
func (l Locale) T(key string) string {
	if v, ok := l[key]; ok {
		return v
	}
	return key
}

// Page returns the title, description and closing text of a page key
// ("404", "4xx", "internal", "maintenance"). Missing parts are empty
// except that missing title/description fall back to their keys.
func (c Catalog) Page(lang, pageKey string) (title, description, closing string) {
	l := c[lang]
	title = l.T("pages." + pageKey + ".title")
	description = l.T("pages." + pageKey + ".description")
	closing = l["pages."+pageKey+".closing"]
	return title, description, closing
}

// Load builds the catalog for langs. Per language it merges, in order:
// locales/{lang}.yaml, themes/{theme}/locales/{lang}.yaml, then the same two
// layouts below operatorDir. Only the base file is required.
func Load(assets fs.FS, theme, operatorDir string, langs []string) (Catalog, error) {
	cat := make(Catalog, len(langs))
	for _, lang := range langs {
		loc := Locale{}
		base := path.Join("locales", lang+".yaml")
		data, err := fs.ReadFile(assets, base)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", base, err)
		}
		if err := merge(loc, data, base); err != nil {
			return nil, err
		}

		rels := []string{path.Join("themes", theme, "locales", lang+".yaml")}
		for _, rel := range rels {
			if err := mergeFS(loc, assets, rel); err != nil {
				return nil, err
			}
		}

		if operatorDir != "" {
			files := []string{
				filepath.Join(operatorDir, "locales", lang+".yaml"),
				filepath.Join(operatorDir, "themes", theme, "locales", lang+".yaml"),
			}
			for _, f := range files {
				data, err := os.ReadFile(f)
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return nil, fmt.Errorf("read %s: %w", f, err)
				}
				if err := merge(loc, data, f); err != nil {
					return nil, err
				}
			}
		}
		cat[lang] = loc
	}
	return cat, nil
}

func mergeFS(loc Locale, assets fs.FS, name string) error {
	data, err := fs.ReadFile(assets, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return merge(loc, data, name)
}

func merge(loc Locale, data []byte, name string) error {
	var tree map[string]any
	if err := yaml.Unmarshal(data, &tree); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	flatten(loc, "", tree)
	return nil
}

func flatten(out Locale, prefix string, node map[string]any) {
	for k, v := range node {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			flatten(out, key, val)
		case string:
			out[key] = val
		case int:
			out[key] = strconv.Itoa(val)
		case bool:
			out[key] = strconv.FormatBool(val)
		case float64:
			out[key] = strconv.FormatFloat(val, 'g', -1, 64)
		}
	}
}
