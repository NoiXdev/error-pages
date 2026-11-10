// Package server is the HTTP handler: it picks the page for a request and
// serves it. Pages are self-contained; the service serves no asset files.
package server

import (
	"bytes"
	crand "crypto/rand"
	"encoding/hex"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
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
	maxAccept    = 32 // Accept entries considered, as for Accept-Language
)

type server struct {
	cfg     config.Config
	renders []*render.Renderer // one per configured theme, never empty
	mode    *maintenance.Mode
	now     func() time.Time
	intn    func(n int) int
	log     *slog.Logger
}

// New returns the handler. rs holds one renderer per configured theme and
// must not be empty; with more than one, every HTML response uses a theme
// drawn by intn, which returns a number in [0, n). nil intn means
// math/rand/v2's IntN. now is the clock for details and Retry-After; nil
// means time.Now.
func New(cfg config.Config, rs []*render.Renderer, m *maintenance.Mode, now func() time.Time, intn func(n int) int) http.Handler {
	if len(rs) == 0 {
		panic("server: no renderer")
	}
	s := &server{cfg: cfg, renders: rs, mode: m, now: now, intn: intn, log: slog.Default()}
	if now == nil {
		s.now = time.Now
	}
	if intn == nil {
		s.intn = rand.IntN
	}
	return s
}

// renderer returns the renderer for a response. Only HTML depends on the
// theme, so only HTML draws one, and only when there is a choice; other
// formats use the first theme.
func (s *server) renderer(f format) *render.Renderer {
	if f != formatHTML || len(s.renders) == 1 {
		return s.renders[0]
	}
	return s.renders[s.intn(len(s.renders))]
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

// choosePage resolves the page from the path, else from X-Code, else the
// default page. A path that names a page always wins, so a client header
// cannot override a page a proxy rewrote the request to; X-Code serves
// proxies that pass the original path (ingress-nginx).
func (s *server) choosePage(req *http.Request) pages.Page {
	if p, ok := pages.FromPath(req.URL.Path); ok {
		return p
	}
	if p, ok := pages.FromCode(req.Header.Get("X-Code")); ok {
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

// offered lists the formats in order of preference for ties.
var offered = [...]struct {
	f        format
	typ, sub string
}{
	{formatHTML, "text", "html"},
	{formatJSON, "application", "json"},
	{formatPlain, "text", "plain"},
}

// negotiateFormat picks the offered format with the highest q-value in an
// Accept header. Each format takes the q of the most specific matching
// range (type/subtype, then type/*, then */*); q=0 excludes it. Ties, an
// empty or unparsable header and no acceptable format all give HTML.
func negotiateFormat(accept string) format {
	var (
		q    [len(offered)]float64
		spec = [len(offered)]int{-1, -1, -1}
	)
	for i, entry := range strings.SplitN(accept, ",", maxAccept+1) {
		if i == maxAccept {
			break
		}
		mt, params, _ := strings.Cut(entry, ";")
		typ, sub, ok := strings.Cut(strings.ToLower(strings.TrimSpace(mt)), "/")
		if !ok {
			continue
		}
		qv := 1.0
		for _, p := range strings.Split(params, ";") {
			k, v, _ := strings.Cut(p, "=")
			if !strings.EqualFold(strings.TrimSpace(k), "q") {
				continue
			}
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil || !(f >= 0 && f <= 1) {
				return formatHTML
			}
			qv = f
		}
		for j, o := range offered {
			level := -1
			switch {
			case typ == o.typ && sub == o.sub:
				level = 2
			case typ == o.typ && sub == "*":
				level = 1
			case typ == "*" && sub == "*":
				level = 0
			}
			if level > spec[j] {
				spec[j], q[j] = level, qv
			}
		}
	}
	best, bestQ := formatHTML, 0.0
	for j, o := range offered {
		if spec[j] >= 0 && q[j] > bestQ {
			best, bestQ = o.f, q[j]
		}
	}
	return best
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
	f := negotiateFormat(req.Header.Get("Accept"))
	r := s.renderer(f)
	switch f {
	case formatJSON:
		ct, err = "application/json; charset=utf-8", r.JSON(&buf, rr)
	case formatPlain:
		ct, err = "text/plain; charset=utf-8", r.Plain(&buf, rr)
	default:
		ct, err = "text/html; charset=utf-8", r.HTML(&buf, rr)
	}
	if err != nil {
		s.log.Error("render failed", "page", page.Key, "theme", r.Theme(), "err", err)
		secure(w.Header())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	secure(h)
	h.Set("Content-Type", ct)
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	h.Set("Content-Security-Policy", r.CSP())
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
	_, _ = crand.Read(b[:]) // crypto/rand does not fail on supported platforms
	return hex.EncodeToString(b[:])
}

// sanitize drops control characters and bidi controls (which could reorder
// the surrounding text) and truncates to limit characters.
func sanitize(s string, limit int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == utf8.RuneError || unicode.Is(unicode.Bidi_Control, r) {
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
