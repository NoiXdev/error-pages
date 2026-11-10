// Package server is the HTTP handler: it picks the page for a request and
// serves it together with the bundled static assets.
package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NoiXdev/error-pages/internal/config"
	"github.com/NoiXdev/error-pages/internal/lang"
	"github.com/NoiXdev/error-pages/internal/maintenance"
	"github.com/NoiXdev/error-pages/internal/pages"
	"github.com/NoiXdev/error-pages/internal/render"
)

const (
	csp = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

	maxHost      = 253
	maxRequestID = 128

	staticCache = "public, max-age=31536000, immutable"
	themesPart  = "themes/"
)

// staticTypes lists the servable static extensions and their content types.
var staticTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".woff2": "font/woff2",
	".txt":   "text/plain; charset=utf-8",
}

type server struct {
	cfg    config.Config
	render *render.Renderer
	mode   *maintenance.Mode
	shared fs.FS // web/static
	themes fs.FS // web/themes/<configured theme>
	now    func() time.Time
	log    *slog.Logger
}

// New returns the handler. assets is the bundled web filesystem; only its
// static directory and the configured theme's directory are ever served.
func New(cfg config.Config, r *render.Renderer, m *maintenance.Mode, assets fs.FS, version string, now func() time.Time) http.Handler {
	s := &server{cfg: cfg, render: r, mode: m, now: now, log: slog.Default()}
	if now == nil {
		s.now = time.Now
	}
	// Sub only fails for invalid paths; these are constants and the
	// configured theme name is validated by config.
	s.shared, _ = fs.Sub(assets, "static")
	s.themes, _ = fs.Sub(assets, path.Join("themes", cfg.Theme))
	_ = version // the version is part of the URL but not checked: any value serves
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	get := req.Method == http.MethodGet || req.Method == http.MethodHead
	switch {
	case get && req.URL.Path == "/healthz":
		s.healthz(w, req)
	case get && strings.HasPrefix(req.URL.Path, "/static/"):
		s.static(w, req)
	default:
		s.page(w, req)
	}
}

func (s *server) healthz(w http.ResponseWriter, req *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		_, _ = w.Write([]byte("ok\n"))
	}
}

// static serves /static/<version>/<file> from web/static, and
// /static/<version>/themes/<theme>/<file> from the configured theme only.
func (s *server) static(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, "/static/")
	_, name, ok := strings.Cut(rest, "/") // drop the version segment
	if !ok || name == "" || path.Clean(name) != name || !fs.ValidPath(name) {
		http.NotFound(w, req)
		return
	}
	ct, ok := staticTypes[strings.ToLower(path.Ext(name))]
	if !ok {
		http.NotFound(w, req)
		return
	}

	fsys := s.shared
	if themed, isTheme := strings.CutPrefix(name, themesPart); isTheme {
		theme, file, ok := strings.Cut(themed, "/")
		if !ok || theme != s.cfg.Theme || file == "" {
			http.NotFound(w, req)
			return
		}
		fsys, name = s.themes, file
	}

	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("Cache-Control", staticCache)
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (s *server) choosePage(req *http.Request) pages.Page {
	if p, ok := pages.FromCode(req.Header.Get("X-Code")); ok {
		return p
	}
	if p, ok := pages.FromPath(req.URL.Path); ok {
		return p
	}
	p, _ := pages.Lookup(s.cfg.DefaultPage) // validated by config
	return p
}

type format int

const (
	formatHTML format = iota
	formatJSON
	formatPlain
)

func negotiateFormat(accept string) format {
	a := strings.ToLower(accept)
	if strings.Contains(a, "text/html") {
		return formatHTML
	}
	switch {
	case strings.Contains(a, "application/json"):
		return formatJSON
	case strings.Contains(a, "text/plain"):
		return formatPlain
	}
	return formatHTML
}

func (s *server) page(w http.ResponseWriter, req *http.Request) {
	page := s.choosePage(req)
	state := s.mode.State()
	if state.On && page.IsServerError() {
		page, _ = pages.Lookup("maintenance")
	}

	status := page.Code
	if !s.cfg.SendSameHTTPCode {
		status = http.StatusOK
	}

	language := lang.Negotiate(req.URL.Query().Get("lang"), req.Header.Get("Accept-Language"), s.cfg.Languages, s.cfg.DefaultLanguage)

	rr := render.Request{Page: page, Lang: language}
	isMaintenance := page.Key == "maintenance"
	if isMaintenance {
		rr.Until = state.Until
	}
	if s.cfg.ShowDetails {
		now := s.now().UTC()
		rr.Details = &render.Details{Time: now, Host: s.host(req), RequestID: s.requestID(req)}
	}

	var (
		buf bytes.Buffer
		ct  string
		err error
	)
	switch negotiateFormat(req.Header.Get("Accept")) {
	case formatJSON:
		ct, err = "application/json; charset=utf-8", s.render.JSON(&buf, rr)
	case formatPlain:
		ct, err = "text/plain; charset=utf-8", s.render.Plain(&buf, rr)
	default:
		ct, err = "text/html; charset=utf-8", s.render.HTML(&buf, rr)
	}
	if err != nil {
		s.log.Error("render failed", "page", page.Key, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Language", language)
	h.Set("Vary", "Accept, Accept-Language")
	if isMaintenance && !state.Until.IsZero() {
		secs := int64((state.Until.Sub(s.now()) + time.Second - 1) / time.Second)
		h.Set("Retry-After", strconv.FormatInt(max(secs, 1), 10))
	}
	w.WriteHeader(status)
	if req.Method != http.MethodHead {
		_, _ = w.Write(buf.Bytes())
	}
}

func (s *server) host(req *http.Request) string {
	h := req.Header.Get("X-Forwarded-Host")
	if first, _, ok := strings.Cut(h, ","); ok {
		h = first
	}
	h = strings.TrimSpace(h)
	if h == "" {
		h = req.Host
	}
	return sanitize(h, maxHost)
}

func (s *server) requestID(req *http.Request) string {
	if id := sanitize(req.Header.Get("X-Request-Id"), maxRequestID); id != "" {
		return id
	}
	var b [6]byte
	_, _ = rand.Read(b[:]) // crypto/rand does not fail on supported platforms
	return hex.EncodeToString(b[:])
}

// sanitize drops control characters and truncates to limit characters.
func sanitize(s string, limit int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == utf8.RuneError {
			continue
		}
		if n == limit {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}
