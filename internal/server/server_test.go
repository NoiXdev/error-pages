package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/NoiXdev/error-pages/internal/config"
	"github.com/NoiXdev/error-pages/internal/maintenance"
	"github.com/NoiXdev/error-pages/internal/render"
	"github.com/NoiXdev/error-pages/internal/texts"
	"github.com/NoiXdev/error-pages/web"
)

var clock = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

type env map[string]string

// newHandler builds a handler; flag is the content of the maintenance file
// (nil means the file does not exist).
func newHandler(t *testing.T, e env, flag *string) http.Handler {
	t.Helper()
	file := filepath.Join(t.TempDir(), "maintenance")
	if flag != nil {
		if err := os.WriteFile(file, []byte(*flag), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e = merge(e, env{"MAINTENANCE_FILE": file})
	cfg, err := config.Load(func(k string) (string, bool) { v, ok := e[k]; return v, ok })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cat, err := texts.Load(web.FS, cfg.Theme, "", cfg.Languages)
	if err != nil {
		t.Fatalf("texts.Load: %v", err)
	}
	now := func() time.Time { return clock }
	r, err := render.New(cfg, cat, web.FS, "1.2.3", now)
	if err != nil {
		t.Fatalf("render.New: %v", err)
	}
	m := maintenance.New(cfg.Maintenance, cfg.MaintenanceUntil, cfg.MaintenanceFile, now)
	return New(cfg, r, m, now)
}

func merge(a, b env) env {
	out := env{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func do(h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRoutes(t *testing.T) {
	h := newHandler(t, nil, nil)
	hInternal := newHandler(t, env{"DEFAULT_PAGE": "internal"}, nil)
	tests := []struct {
		h     http.Handler
		path  string
		code  int
		title string
	}{
		{h, "/503.html", 503, "503"},
		{h, "/internal", 403, "Available internally only"},
		{h, "/maintenance", 503, "maintenance"},
		{h, "/foo", 404, "Page not found"},
		{hInternal, "/foo", 403, "Available internally only"},
	}
	for _, tc := range tests {
		rec := do(tc.h, "GET", tc.path, nil)
		if rec.Code != tc.code {
			t.Errorf("%s: status %d, want %d", tc.path, rec.Code, tc.code)
		}
		if !strings.Contains(rec.Body.String(), tc.title) {
			t.Errorf("%s: body lacks %q", tc.path, tc.title)
		}
	}
}

func TestOddPaths(t *testing.T) {
	h := newHandler(t, nil, nil)
	for _, p := range []string{"/0503", "/999", "/200", "/404/", "/%2e%2e/etc", "/" + strings.Repeat("a", 4096)} {
		rec := do(h, "GET", p, nil)
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), "Page not found") {
			t.Errorf("%.20s: status %d, want 404 page", p, rec.Code)
		}
	}
	if rec := do(h, "GET", "/503.html?x=1", nil); rec.Code != 503 {
		t.Errorf("query: status %d, want 503", rec.Code)
	}
}

// TestNoStaticAssets: the service serves no files. Former asset paths get
// the normal 404 page with the page headers.
func TestNoStaticAssets(t *testing.T) {
	h := newHandler(t, nil, nil)
	for _, p := range []string{"/static/1.2.3/css/base.css", "/static/x/js/i18n.js",
		"/static/1.2.3/fonts/inter-latin-wght-normal.woff2", "/static/../go.mod"} {
		rec := do(h, "GET", p, nil)
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), "Page not found") {
			t.Errorf("%s: status %d, want the 404 page", p, rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("%s: page headers missing", p)
		}
	}
}

func TestXCode(t *testing.T) {
	h := newHandler(t, nil, nil)
	if rec := do(h, "GET", "/404.html", map[string]string{"X-Code": "502"}); rec.Code != 502 {
		t.Errorf("X-Code 502: %d", rec.Code)
	}
	if rec := do(h, "GET", "/404.html", map[string]string{"X-Code": "abc"}); rec.Code != 404 {
		t.Errorf("X-Code abc: %d", rec.Code)
	}
}

func TestMaintenanceOverride(t *testing.T) {
	on := ""
	h := newHandler(t, nil, &on)
	for path, want := range map[string]int{"/502.html": 503, "/404.html": 404, "/internal.html": 403} {
		rec := do(h, "GET", path, nil)
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
		if path == "/502.html" && !strings.Contains(rec.Body.String(), `data-page="maintenance"`) {
			t.Errorf("502 not turned into maintenance page")
		}
		if rec.Header().Get("Retry-After") != "" {
			t.Errorf("%s: Retry-After without until", path)
		}
	}
	until := clock.Add(90 * time.Second).Format(time.RFC3339)
	h = newHandler(t, nil, &until)
	rec := do(h, "GET", "/502.html", nil)
	if got := rec.Header().Get("Retry-After"); got != "90" {
		t.Errorf("Retry-After %q, want 90", got)
	}
	if !strings.Contains(rec.Body.String(), "2026-11-10 12:01 UTC") {
		t.Errorf("until text missing")
	}
	if got := do(h, "GET", "/404.html", nil).Header().Get("Retry-After"); got != "" {
		t.Errorf("Retry-After on non-maintenance page: %q", got)
	}
}

func TestRetryAfterRoundsUp(t *testing.T) {
	until := clock.Add(90*time.Second + 400*time.Millisecond).Format(time.RFC3339Nano)
	h := newHandler(t, nil, &until)
	if got := do(h, "GET", "/503", nil).Header().Get("Retry-After"); got != "91" {
		t.Errorf("Retry-After %q, want 91", got)
	}
}

func TestSendSameHTTPCodeFalse(t *testing.T) {
	h := newHandler(t, env{"SEND_SAME_HTTP_CODE": "false"}, nil)
	if rec := do(h, "GET", "/503", nil); rec.Code != 200 {
		t.Errorf("status %d, want 200", rec.Code)
	}
}

func TestFormats(t *testing.T) {
	h := newHandler(t, nil, nil)
	rec := do(h, "GET", "/503", map[string]string{"Accept": "Application/JSON"})
	var v struct{ Code int }
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || v.Code != 503 {
		t.Errorf("json: %v %q", err, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("json content type %q", ct)
	}
	rec = do(h, "GET", "/503", map[string]string{"Accept": "text/plain"})
	if !strings.HasPrefix(rec.Body.String(), "503 ") || strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("plain: %q", rec.Body.String())
	}
	rec = do(h, "GET", "/503", map[string]string{"Accept": "text/html,application/json"})
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("html expected")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("html content type %q", ct)
	}
}

func TestHeaders(t *testing.T) {
	h := newHandler(t, nil, nil)
	rec := do(h, "GET", "/503", map[string]string{"Accept-Language": "pl"})
	want := map[string]string{
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"X-Frame-Options":        "DENY",
		"Content-Language":       "pl",
		"Vary":                   "Accept, Accept-Language",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if _, ok := rec.Header()["Server"]; ok {
		t.Error("Server header set")
	}
}

var (
	styleBlock  = regexp.MustCompile(`(?s)<style>(.*?)</style>`)
	scriptBlock = regexp.MustCompile(`(?s)<script>(.*?)</script>`)
)

// markup strips the inline CSS and JS from a page.
func markup(body string) string {
	return scriptBlock.ReplaceAllString(styleBlock.ReplaceAllString(body, ""), "")
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// TestCSP checks for every theme that the header allows exactly the page's
// inline style and script, and that JSON and plain text get the same policy.
func TestCSP(t *testing.T) {
	for _, theme := range config.Themes {
		t.Run(theme, func(t *testing.T) {
			h := newHandler(t, env{"THEME": theme}, nil)
			rec := do(h, "GET", "/maintenance", map[string]string{"Accept-Language": "de"})
			body := rec.Body.String()
			css, js := styleBlock.FindAllStringSubmatch(body, -1), scriptBlock.FindAllStringSubmatch(body, -1)
			if len(css) != 1 || len(js) != 1 {
				t.Fatalf("%d style and %d script blocks", len(css), len(js))
			}
			want := "default-src 'none'; img-src data:; font-src data:; style-src 'sha256-" + sha(css[0][1]) +
				"'; script-src 'sha256-" + sha(js[0][1]) + "'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
			if got := rec.Header().Get("Content-Security-Policy"); got != want {
				t.Errorf("CSP = %q\nwant  %q", got, want)
			}
			for _, accept := range []string{"application/json", "text/plain"} {
				if got := do(h, "GET", "/404", map[string]string{"Accept": accept}).Header().Get("Content-Security-Policy"); got != want {
					t.Errorf("%s: CSP %q", accept, got)
				}
			}
		})
	}
}

func TestHead(t *testing.T) {
	h := newHandler(t, nil, nil)
	get := do(h, "GET", "/503", nil)
	head := do(h, "HEAD", "/503", nil)
	if head.Code != get.Code || head.Body.Len() != 0 {
		t.Errorf("HEAD: %d, %d body bytes", head.Code, head.Body.Len())
	}
	if head.Header().Get("Content-Security-Policy") != get.Header().Get("Content-Security-Policy") {
		t.Error("HEAD lacks headers")
	}
	if rec := do(h, "POST", "/503", nil); rec.Code != 503 {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	h := newHandler(t, nil, nil)
	rec := do(h, "GET", "/healthz", nil)
	if rec.Code != 200 || rec.Body.String() != "ok\n" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("healthz: %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
}

func TestLangQuery(t *testing.T) {
	h := newHandler(t, nil, nil)
	if rec := do(h, "GET", "/404?lang=pl", nil); !strings.Contains(rec.Body.String(), `<html lang="pl"`) {
		t.Error("lang=pl not honoured")
	}
}

func TestDetails(t *testing.T) {
	h := newHandler(t, env{"THEME": "status"}, nil)
	rec := do(h, "GET", "/404", map[string]string{
		"X-Request-Id":     "abc\r\n" + strings.Repeat("z", 200),
		"X-Forwarded-Host": "exa\tmple.org",
	})
	if rec.Code != 404 {
		t.Fatal(rec.Code)
	}
	body := rec.Body.String()
	// Control characters are dropped and the ID is cut to 128 characters.
	if strings.ContainsAny(body, "\r\t") {
		t.Error("control character in body")
	}
	id := "abc" + strings.Repeat("z", 125)
	want := `<p class="s-meta">2026-11-10 12:00:00 UTC · example.org · ` + id + `</p>`
	if !strings.Contains(body, want) {
		t.Errorf("body lacks %q", want)
	}
}

func TestDetailsHidden(t *testing.T) {
	h := newHandler(t, env{"THEME": "status", "SHOW_DETAILS": "false"}, nil)
	body := do(h, "GET", "/502", map[string]string{"X-Forwarded-Host": "example.org", "X-Request-Id": "abc"}).Body.String()
	if m := markup(body); strings.Contains(m, "s-meta") || strings.Contains(m, "example.org") {
		t.Error("details shown with SHOW_DETAILS=false")
	}
}

func TestDetailsFirstForwardedHost(t *testing.T) {
	h := newHandler(t, env{"THEME": "status"}, nil)
	body := do(h, "GET", "/502", map[string]string{"X-Forwarded-Host": "a.example, b.example", "X-Request-Id": "abc"}).Body.String()
	if !strings.Contains(body, " · a.example · abc</p>") {
		t.Error("first forwarded host not shown")
	}
	if strings.Contains(body, "b.example") {
		t.Error("second forwarded host leaked")
	}
}

func TestDetailsTruncated(t *testing.T) {
	h := newHandler(t, env{"THEME": "status"}, nil)
	body := do(h, "GET", "/502", map[string]string{"X-Forwarded-Host": strings.Repeat("h", 2048), "X-Request-Id": "abc"}).Body.String()
	if !strings.Contains(body, " · "+strings.Repeat("h", 253)+" · abc</p>") {
		t.Error("host not shown truncated to 253 characters")
	}
	if strings.Contains(body, strings.Repeat("h", 254)) {
		t.Error("host longer than 253 characters")
	}
}

func TestSendSameHTTPCodeFalseJSON(t *testing.T) {
	h := newHandler(t, env{"SEND_SAME_HTTP_CODE": "false"}, nil)
	rec := do(h, "GET", "/503", map[string]string{"Accept": "application/json"})
	if rec.Code != 200 {
		t.Errorf("status %d, want 200", rec.Code)
	}
	var v struct{ Code int }
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || v.Code != 503 {
		t.Errorf("json body: %v %q", err, rec.Body.String())
	}
}

func TestRetryAfterPastUntil(t *testing.T) {
	until := clock.Add(-time.Hour).Format(time.RFC3339)
	h := newHandler(t, nil, &until)
	rec := do(h, "GET", "/502", nil)
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), `data-page="maintenance"`) {
		t.Fatalf("maintenance not active: %d", rec.Code)
	}
	if strings.Contains(markup(rec.Body.String()), "data-until") {
		t.Error("end time shown although it is in the past")
	}
	if got := rec.Header().Get("Retry-After"); got != "" {
		t.Errorf("Retry-After %q for an end time in the past", got)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("a\x00b\r\nc\x7fd", 128); got != "abcd" {
		t.Errorf("sanitize = %q", got)
	}
	if got := sanitize(strings.Repeat("é", 300), 253); len([]rune(got)) != 253 {
		t.Errorf("truncate = %d runes", len([]rune(got)))
	}
}

func TestErrorAndHealthzHeaders(t *testing.T) {
	h := newHandler(t, nil, nil)
	rec := do(h, "GET", "/healthz", nil)
	for k, v := range map[string]string{"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer", "X-Frame-Options": "DENY"} {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("healthz %s = %q, want %q", k, got, v)
		}
	}
}
