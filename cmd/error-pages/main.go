// Command error-pages serves Traefik error and maintenance pages.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/NoiXdev/error-pages/internal/config"
	"github.com/NoiXdev/error-pages/internal/maintenance"
	"github.com/NoiXdev/error-pages/internal/render"
	"github.com/NoiXdev/error-pages/internal/server"
	"github.com/NoiXdev/error-pages/internal/texts"
	"github.com/NoiXdev/error-pages/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: error-pages [command]

commands:
  serve                                    run the HTTP server (default)
  healthcheck                              probe /healthz on the local listener
  maintenance on [--until RFC3339] | off | status
  version                                  print the version
`

func main() {
	os.Exit(run(os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) (string, bool), stdout, stderr io.Writer) int {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return serve(getenv, stderr)
	case "healthcheck":
		return healthcheck(getenv, stderr)
	case "maintenance":
		return maintenanceCmd(args, getenv, stdout, stderr)
	case "version":
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n%s", cmd, usage)
		return 2
	}
}

func serve(getenv func(string) (string, bool), stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return serveCtx(ctx, getenv, stderr, nil)
}

// serveCtx runs the server until ctx is done, then shuts it down. ready,
// if not nil, is called with the bound address once the server listens.
func serveCtx(ctx context.Context, getenv func(string) (string, bool), stderr io.Writer, ready func(net.Addr)) int {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fail(stderr, err)
	}
	catalog := func(theme string) (texts.Catalog, error) {
		return texts.Load(web.FS, theme, cfg.TextsDir, cfg.Languages)
	}
	rs, err := render.NewThemes(cfg, catalog, web.FS, version, time.Now)
	if err != nil {
		return fail(stderr, err)
	}
	m := maintenance.New(cfg.Maintenance, cfg.MaintenanceUntil, cfg.MaintenanceFile, time.Now)

	srv := newHTTPServer(server.New(cfg, rs, m, time.Now, nil))
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fail(stderr, err)
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	log.Info("starting", "themes", cfg.Themes, "languages", cfg.Languages,
		"listen", ln.Addr().String(), "version", version)
	if ready != nil {
		ready(ln.Addr())
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return fail(stderr, err)
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		return fail(stderr, err)
	}
	return 0
}

// newHTTPServer returns the server with explicit limits on every phase of a
// connection, so a slow or idle client cannot hold one open indefinitely.
func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

// healthURL builds the probe URL for a LISTEN value. An empty or unspecified
// host maps to the loopback address.
func healthURL(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", err
	}
	if host == "" {
		host = "127.0.0.1"
	} else if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz", nil
}

func healthcheck(getenv func(string) (string, bool), stderr io.Writer) int {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fail(stderr, err)
	}
	u, err := healthURL(cfg.Listen)
	if err != nil {
		return fail(stderr, err)
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		// A redirect is not healthy: judge the first response only.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(u)
	if err != nil {
		return fail(stderr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fail(stderr, fmt.Errorf("healthcheck: status %d", resp.StatusCode))
	}
	return 0
}

func maintenanceCmd(args []string, getenv func(string) (string, bool), stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	action, args := args[0], args[1:]
	var until string
	switch action {
	case "on":
		fs := flag.NewFlagSet("maintenance on", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.StringVar(&until, "until", "", "")
		if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
			_, _ = fmt.Fprint(stderr, usage)
			return 2
		}
	case "off", "status":
		if len(args) != 0 {
			_, _ = fmt.Fprint(stderr, usage)
			return 2
		}
	default:
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		return fail(stderr, err)
	}
	if action != "status" {
		if err := maintenance.Switch(cfg.MaintenanceFile, action == "on", until); err != nil {
			return fail(stderr, err)
		}
	}
	m := maintenance.New(cfg.Maintenance, cfg.MaintenanceUntil, cfg.MaintenanceFile, time.Now)
	_, _ = fmt.Fprintln(stdout, maintenance.Describe(m))
	return 0
}

// fail prints err on one line to stderr and returns exit code 1.
func fail(stderr io.Writer, err error) int {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	_, _ = fmt.Fprintln(stderr, msg)
	return 1
}
