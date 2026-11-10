// Package server is the HTTP handler: it picks the page for a request and
// serves it. Pages are self-contained; the service serves no asset files.
package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
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
	maxHost      = 253
	maxRequestID = 128
)

type server struct {
	cfg    config.Config
	render *render.Renderer
	mode   *maintenance.Mode
	now    func() time.Time
	log    *slog.Logger
}

// New returns the handler. now is the clock for details and Retry-After;
// nil means time.Now.
func New(cfg config.Config, r *render.Renderer, m *maintenance.Mode, now func() time.Time) http.Handler {
	s := &server{cfg: cfg, render: r, mode: m, now: now, log: slog.Default()}
	if now == nil {
		s.now = time.Now
	}
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	get := req.Method == http.MethodGet || req.Method == http.MethodHead
	if get && req.URL.Path == "/healthz" {
		s.healthz(w, req)
		return
	}
	s.page(w, req)
}

// secure sets the headers every response carries.
func secure(h http.Header) {
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
}

func (s *server) healthz(w http.ResponseWriter, req *http.Request) {
	h := w.Header()
	secure(h)
	h.Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		_, _ = w.Write([]byte("ok\n"))
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
		secure(w.Header())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	secure(h)
	h.Set("Content-Type", ct)
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	h.Set("Content-Security-Policy", s.render.CSP())
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
