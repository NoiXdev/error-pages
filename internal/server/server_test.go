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
	return newHandlerPick(t, e, flag, nil)
}

// newHandlerPick is newHandler with the theme pick source intn.
func newHandlerPick(t *testing.T, e env, flag *string, intn func(int) int) http.Handler {
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
	catalog := func(theme string) (texts.Catalog, error) { return texts.Load(web.FS, theme, "", cfg.Languages) }
	now := func() time.Time { return clock }
	rs, err := render.NewThemes(cfg, catalog, web.FS, "1.2.3", now)
	if err != nil {
		t.Fatalf("render.NewThemes: %v", err)
	}
	m := maintenance.New(cfg.Maintenance, cfg.MaintenanceUntil, cfg.MaintenanceFile, now)
	return New(cfg, rs, m, now, intn)
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
	for _, tc := range []struct {
		path, code string
		want       int
	}{
		// The path does not name a page (ingress-nginx sends the original path).
		{"/api/users", "502", 502},
		{"/", "429", 429},
		{"/foo", "abc", 404},
		// A path naming a page wins over the header.
		{"/404.html", "502", 404},
		{"/internal.html", "200", 403},
		{"/internal.html", "503", 403},
		{"/503", "404", 503},
		{"/499", "502", 499},
	} {
		if rec := do(h, "GET", tc.path, map[string]string{"X-Code": tc.code}); rec.Code != tc.want {
			t.Errorf("%s with X-Code %s: %d, want %d", tc.path, tc.code, rec.Code, tc.want)
		}
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
	if got := sanitize("a\u202eb\u2066c\u200fd\u061ce", 128); got != "abcde" {
		t.Errorf("bidi controls kept: %q", got)
	}
	if got := sanitize(strings.Repeat("é", 300), 253); len([]rune(got)) != 253 {
		t.Errorf("truncate = %d runes", len([]rune(got)))
	}
}

func TestNegotiateFormat(t *testing.T) {
	for accept, want := range map[string]format{
		"":                                formatHTML,
		"text/html":                       formatHTML,
		"application/json":                formatJSON,
		"Application/JSON":                formatJSON,
		"text/plain":                      formatPlain,
		"text/html,application/json":      formatHTML, // tie goes to HTML
		"application/json, text/html;q=0": formatJSON, // q=0 excludes
		"application/json;q=0.9, text/html;q=0.8":   formatJSON,
		"text/html;q=0.5, application/json":         formatJSON,
		"text/plain, application/json;q=0.5":        formatPlain,
		"*/*":                                       formatHTML,
		"application/*":                             formatJSON,
		"text/*":                                    formatHTML,
		"text/*;q=0.1, text/plain":                  formatPlain,
		"text/html;q=0, */*":                        formatJSON, // */* matches json at q=1 next
		"application/json;q=0, text/plain;q=0":      formatHTML, // nothing acceptable: HTML
		"image/png":                                 formatHTML,
		"application/json;q=abc":                    formatHTML, // unparsable q
		";;;,,,":                                    formatHTML,
		"application/json ; q=1.0 ; charset=utf-8":  formatJSON,
		"text/html;level=1;q=0.2, text/plain;q=0.3": formatPlain,
	} {
		if got := negotiateFormat(accept); got != want {
			t.Errorf("%q: %d, want %d", accept, got, want)
		}
	}
	// Entries past the cap are ignored.
	long := strings.Repeat("image/png, ", 32) + "application/json"
	if got := negotiateFormat(long); got != formatHTML {
		t.Errorf("entry past the cap honoured: %d", got)
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

var bodyTheme = regexp.MustCompile(`<body class="ep ep-theme-([a-z]+)">`)

// checkCSP fails unless rec's CSP allows exactly its own inline blocks.
func checkCSP(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
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
}

// seq returns a pick source that yields picks in turn and records each n.
func seq(picks []int, ns *[]int) func(int) int {
	i := 0
	return func(n int) int {
		*ns = append(*ns, n)
		p := picks[i%len(picks)]
		i++
		return p
	}
}

func TestRandomThemePerRequest(t *testing.T) {
	var ns []int
	h := newHandlerPick(t, env{"THEME": "glass,terminal,status"}, nil, seq([]int{1, 2}, &ns))
	for i, want := range []string{"terminal", "status"} {
		rec := do(h, "GET", "/maintenance", map[string]string{"Accept-Language": "de"})
		m := bodyTheme.FindStringSubmatch(rec.Body.String())
		if m == nil || m[1] != want {
			t.Fatalf("request %d: theme %v, want %s", i, m, want)
		}
		checkCSP(t, rec)
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("request %d: Cache-Control = %q", i, got)
		}
		if rec.Code != 503 {
			t.Errorf("request %d: status %d", i, rec.Code)
		}
	}
	if len(ns) != 2 || ns[0] != 3 || ns[1] != 3 {
		t.Errorf("pick source called with %v, want [3 3]", ns)
	}
}

func TestRandomThemeAll(t *testing.T) {
	var ns []int
	picks := make([]int, len(config.Themes))
	for i := range picks {
		picks[i] = i
	}
	h := newHandlerPick(t, env{"THEME": "random"}, nil, seq(picks, &ns))
	for _, want := range config.Themes {
		rec := do(h, "GET", "/404", nil)
		if m := bodyTheme.FindStringSubmatch(rec.Body.String()); m == nil || m[1] != want {
			t.Fatalf("theme %v, want %s", m, want)
		}
		checkCSP(t, rec)
	}
}

// TestRandomThemeNonHTML checks that JSON and plain responses draw no theme
// and are the same as with a single theme.
func TestRandomThemeNonHTML(t *testing.T) {
	var ns []int
	multi := newHandlerPick(t, env{"THEME": "terminal,glass"}, nil, seq([]int{1}, &ns))
	single := newHandler(t, env{"THEME": "terminal"}, nil)
	for _, accept := range []string{"application/json", "text/plain"} {
		a := do(multi, "GET", "/502", map[string]string{"Accept": accept})
		b := do(single, "GET", "/502", map[string]string{"Accept": accept})
		if a.Body.String() != b.Body.String() || a.Code != b.Code {
			t.Errorf("%s: %d %q vs %d %q", accept, a.Code, a.Body, b.Code, b.Body)
		}
		for _, k := range []string{"Content-Type", "Content-Security-Policy", "Cache-Control"} {
			if a.Header().Get(k) != b.Header().Get(k) {
				t.Errorf("%s: %s differs: %q vs %q", accept, k, a.Header().Get(k), b.Header().Get(k))
			}
		}
	}
	if len(ns) != 0 {
		t.Errorf("pick source called %d times for non-HTML responses", len(ns))
	}
}

// TestSingleThemeDrawsNothing is the single-theme regression: the pick
// source is never called and every page uses the one theme.
func TestSingleThemeDrawsNothing(t *testing.T) {
	never := func(int) int { t.Fatal("pick source called with a single theme"); return 0 }
	h := newHandlerPick(t, env{"THEME": "classic"}, nil, never)
	for _, target := range []string{"/404", "/maintenance", "/500"} {
		rec := do(h, "GET", target, nil)
		if m := bodyTheme.FindStringSubmatch(rec.Body.String()); m == nil || m[1] != "classic" {
			t.Fatalf("%s: theme %v", target, m)
		}
		checkCSP(t, rec)
	}
	do(h, "GET", "/404", map[string]string{"Accept": "application/json"})
}
