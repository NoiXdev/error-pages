package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func runCmd(t *testing.T, e map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, env(e), &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunVersion(t *testing.T) {
	code, out, _ := runCmd(t, nil, "version")
	if code != 0 || strings.TrimSpace(out) != version {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRunUnknown(t *testing.T) {
	code, _, errOut := runCmd(t, nil, "bogus")
	if code != 2 || !strings.Contains(errOut, "usage") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestRunBadConfig(t *testing.T) {
	code, _, errOut := runCmd(t, map[string]string{"THEME": "nope"}, "serve")
	if code != 1 || !strings.Contains(errOut, "config: THEME") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestRunMaintenance(t *testing.T) {
	e := map[string]string{"MAINTENANCE_FILE": filepath.Join(t.TempDir(), "maintenance")}
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"maintenance", "on"}, ""},
		{[]string{"maintenance", "status"}, "on"},
		{[]string{"maintenance", "on", "--until", "2099-01-01T00:00:00Z"}, ""},
		{[]string{"maintenance", "status"}, "on until 2099-01-01T00:00:00Z"},
		{[]string{"maintenance", "off"}, ""},
		{[]string{"maintenance", "status"}, "off"},
	}
	for _, s := range steps {
		code, out, errOut := runCmd(t, e, s.args...)
		if code != 0 {
			t.Fatalf("%v: code=%d stderr=%q", s.args, code, errOut)
		}
		if s.want != "" && strings.TrimSpace(out) != s.want {
			t.Fatalf("%v: out=%q want %q", s.args, out, s.want)
		}
	}
}

func TestRunMaintenanceUsage(t *testing.T) {
	e := map[string]string{"MAINTENANCE_FILE": filepath.Join(t.TempDir(), "m")}
	for _, args := range [][]string{{"maintenance"}, {"maintenance", "bogus"}} {
		if code, _, _ := runCmd(t, e, args...); code != 2 {
			t.Fatalf("%v: code=%d, want 2", args, code)
		}
	}
	if code, _, _ := runCmd(t, e, "maintenance", "on", "--until", "garbage"); code != 1 {
		t.Fatalf("bad until: code=%d, want 1", code)
	}
}

func TestRunHealthcheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
		}
	}))
	u, _ := url.Parse(srv.URL)
	if code, _, errOut := runCmd(t, map[string]string{"LISTEN": u.Host}, "healthcheck"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	srv.Close()
	if code, _, _ := runCmd(t, map[string]string{"LISTEN": u.Host}, "healthcheck"); code != 1 {
		t.Fatalf("closed: code=%d, want 1", code)
	}
	if code, _, errOut := runCmd(t, map[string]string{"THEME": "nope"}, "healthcheck"); code != 1 || errOut == "" {
		t.Fatalf("bad config: code=%d stderr=%q", code, errOut)
	}
}

func TestHealthURL(t *testing.T) {
	for in, want := range map[string]string{
		":8080":        "http://127.0.0.1:8080/healthz",
		"0.0.0.0:8080": "http://127.0.0.1:8080/healthz",
		"[::]:9":       "http://127.0.0.1:9/healthz",
		"10.1.2.3:80":  "http://10.1.2.3:80/healthz",
		"[::1]:8080":   "http://[::1]:8080/healthz",
	} {
		got, err := healthURL(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := healthURL("nonsense"); err == nil {
		t.Error("want error for missing port")
	}
}

func TestServeCtxStartsAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := make(chan net.Addr, 1)
	var errb bytes.Buffer
	done := make(chan int, 1)
	e := env(map[string]string{"LISTEN": "127.0.0.1:0", "MAINTENANCE_FILE": filepath.Join(t.TempDir(), "m")})
	go func() { done <- serveCtx(ctx, e, &errb, func(a net.Addr) { addr <- a }) }()

	var a net.Addr
	select {
	case a = <-addr:
	case code := <-done:
		t.Fatalf("serveCtx returned %d before listening: %s", code, errb.String())
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	resp, err := http.Get("http://" + a.String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %d", resp.StatusCode)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code %d: %s", code, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not shut down")
	}
	if !strings.Contains(errb.String(), `"msg":"starting"`) || !strings.Contains(errb.String(), a.String()) {
		t.Errorf("no starting log with the bound address: %s", errb.String())
	}
}

func TestServeCtxListenFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	var errb bytes.Buffer
	e := env(map[string]string{"LISTEN": ln.Addr().String(), "MAINTENANCE_FILE": filepath.Join(t.TempDir(), "m")})
	if code := serveCtx(context.Background(), e, &errb, nil); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
	if strings.Contains(errb.String(), "starting") {
		t.Errorf("logged starting although listen failed: %s", errb.String())
	}
}

func TestHealthcheckDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			http.Redirect(w, r, "/ok", http.StatusFound)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	if code, _, _ := runCmd(t, map[string]string{"LISTEN": u.Host}, "healthcheck"); code != 1 {
		t.Fatalf("redirect: code %d, want 1", code)
	}
}
