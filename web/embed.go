// Package web embeds the built-in locales, themes, templates and static assets.
package web

import "embed"

// FS holds the embedded assets.
//
//go:embed locales themes templates static
var FS embed.FS
