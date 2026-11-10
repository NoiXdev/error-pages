# CLAUDE.md

Guidance for working in this repository.

## What this is

`error-pages` serves HTML, JSON and plain-text error pages for reverse proxies (for
example Traefik's errors middleware). It is a single Go binary with every asset
embedded, shipped as a `scratch` Docker image (`noixdev/error-pages`). Module path:
`github.com/NoiXdev/error-pages`.

## Layout

- `cmd/error-pages/`: CLI. Subcommands: `serve` (default), `healthcheck`,
  `maintenance on [--until RFC3339] | off | status`, `version`.
- `internal/config/`: defaults, optional YAML (`CONFIG_FILE`), then environment
  variables (`THEME`, `LANGUAGES`, `DEFAULT_LANGUAGE`, `DEFAULT_PAGE`,
  `SEND_SAME_HTTP_CODE`, `SHOW_DETAILS`, `SHOW_FOOTER`, `SHOW_VERSION`,
  `COPYRIGHT_NAME`, `COPYRIGHT_URL`, `LINKS`, `MAINTENANCE`, `MAINTENANCE_FILE`,
  `MAINTENANCE_UNTIL`, `TEXTS_DIR`, `LISTEN`). Validation lives here.
- `internal/pages/`: page registry (codes, class pages `4xx`/`5xx`, `internal`,
  `maintenance`).
- `internal/lang/`: language negotiation (`?lang=`, `Accept-Language`, default).
- `internal/texts/`: text catalogue (built-in locales, theme locales, operator
  overrides from `TEXTS_DIR`).
- `internal/maintenance/`: maintenance flag file and `Switch`.
- `internal/render/`: HTML, JSON and plain-text rendering. `inline.go` builds the
  configured theme's inline CSS/JS and the matching CSP once at startup; `themeFonts`
  lists the bundled fonts each theme uses.
- `internal/server/`: HTTP handler: `/<code>.html`, `/healthz`, content negotiation
  (q-aware `Accept`), security headers. No asset files are served.
- `web/`: embedded assets: `locales/` (all page texts), `templates/layout.html.tmpl`,
  `themes/<theme>/` (template, CSS, optional JS), `static/` (shared CSS, JS, fonts with
  one `@font-face` stylesheet per family).

## Commands

```bash
make test    # go test ./...
make lint    # golangci-lint run ./...
make run     # go run the server
make build   # bin/error-pages
```

The version comes from `version.json` (`make`, the Dockerfile and the CI smoke test all
read it).

## Conventions

- English for everything code-side: code, comments, commit messages, PR text, docs.
  User-visible strings live in `web/locales/*.yaml`, never in Go or templates.
- Conventional Commits (`feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`,
  `ci`, `chore`) with the footer `Co-Authored-By: Claude ...`. Commit and push only
  when asked.
- `docs/` (specs, plans, mockups) is local-only and is never committed. Likewise
  `.superpowers/`. Both are in `.gitignore`; never stage them.
- New logic comes with tests. Run `make test lint` before committing.

## Constraints

- Pages are self-contained: one request, nothing else fetched (behind a proxy every
  further request could get an error page instead of the asset). Each page carries
  one `<style>` (the theme's fonts as `data:` URIs, `base.css`, the theme's
  `style.css`) and one `<script>` (`i18n.js` plus the theme's `theme.js`). Both are
  built once at startup and must not contain anything request-specific. Sources stay
  separate files; never copy CSS or JS into templates.
- Strict CSP: `default-src 'none'; img-src data:; font-src data:; style-src
  'sha256-<css>'; script-src 'sha256-<js>'; base-uri 'none'; form-action 'none';
  frame-ancestors 'none'`. No `style=` attributes, no inline event handlers, no
  `javascript:` URLs. Back and Reload are links with `data-action` handled by shared
  JS; without JS they link to `/` and `""`.
- All assets (fonts, CSS, JS, templates, locales) are embedded in the image. No CDN,
  no external fonts, no third-party requests, no cookies; localStorage only for the
  language choice.
- Page selection: a path naming a page wins; `X-Code` applies only when it does not.
- Every page works without JavaScript, from 320 px wide, and honours
  `prefers-reduced-motion: reduce` (no motion, final state at once).
- Leak no internals: no upstream addresses, no server software names, no `Server`
  header.
- Public texts (README, docs intro) name no implementation tech and no
  infrastructure of ours.

## Adding a theme

Create `web/themes/<name>/` with `page.html.tmpl` and `style.css` (plus `theme.js` if
needed), add the name to `Themes` in `internal/config/config.go` and its fonts to
`themeFonts` in `internal/render/inline.go`, add optional
theme texts under `web/themes/<name>/locales/`, and let `internal/render` matrix tests
cover it. Add a screenshot in `.github/assets/screenshots/` and mention it in the
README.

## Release flow

`.github/workflows/release.yaml` is dispatched manually from `main` (`bump`,
`prerelease_id` default `rc`, `dry_run`). It gates on `ci.yaml`, then calls the shared
`create_docker_changelog_version_release.yaml`, which rewrites `version.json`, builds
and pushes `linux/amd64,linux/arm64`, tags and releases. The first release is
`bump=major` with `prerelease_id=rc`, giving `1.0.0-rc.1`. Needs
`vars.DOCKERHUB_NAMESPACE=noixdev`, `vars.DOCKERHUB_IMAGE=error-pages`, and secrets
`DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` (optional: `DISCORD_WEBHOOK_RELEASE_NOTIFICATION`).
GitHub Actions are pinned to full commit SHAs; Docker base images are referenced by tag
(`golang:1.27-alpine`, `scratch`), not by digest. The Dockerfile cross-compiles on
`$BUILDPLATFORM`.
