# Contributing

Thanks for your interest in error-pages! This short guide helps you get started.

## Setup

error-pages is a single Go program with all assets embedded. You need a recent Go
toolchain (see `go.mod`) and [golangci-lint](https://golangci-lint.run/) v2. Docker
is optional and only needed to build the image.

```bash
make test    # go test ./...
make lint    # golangci-lint run ./...
make run     # start the server on :8080
make build   # build bin/error-pages
```

Open <http://localhost:8080/502.html> to see a page. `THEME=terminal make run` picks
another theme.

## Guidelines

- Write code, comments, commit messages and documentation in English. Visible page
  text lives in `web/locales/*.yaml`, one file per language.
- New logic comes with tests. Run `make test lint` before you push.
- Pages must work without JavaScript, from 320 px wide, and honour
  `prefers-reduced-motion: reduce` (no movement, final state shown at once).
- The Content Security Policy is strict: no inline scripts, no `style=` attributes and
  no `javascript:` URLs. Back and Reload are links with a `data-action` attribute that
  the shared script handles.
- Pages make no third-party requests: no CDN and no external fonts.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`). The
  release notes are generated from them.
- Keep the history linear: rebase on `main` instead of merging it into your branch.

## Adding a theme

1. Create `web/themes/<name>/` with `page.html.tmpl` and `style.css` (and a
   `theme.js` if the theme needs one). Look at an existing theme for the template
   variables.
2. Add the name to `Themes` in `internal/config/config.go`.
3. Theme-specific texts go in `web/themes/<name>/locales/<lang>.yaml`.
4. The render tests in `internal/render` iterate over every theme, page and
   language, so they cover the new theme as soon as it is listed.

## Adding or changing a translation

Edit `web/locales/<lang>.yaml`. To add a language, add its file and extend the allowed
language list in `internal/config/config.go`.

## Releasing

Releases are cut by the **Release** workflow (`.github/workflows/release.yaml`), run
from `main` with *Run workflow*. It runs the CI gates on the commit, then calls the
shared release workflow of the organisation: it computes the next version from
`version.json`, writes it into the tree, builds and pushes a multi-architecture image
(`linux/amd64`, `linux/arm64`), commits the version bump, tags, creates the GitHub
release and generates the changelog from Conventional Commits.

Inputs:

- `bump`: `patch`, `minor` or `major`.
- `prerelease_id`: defaults to `rc`; leave it empty to promote a candidate to a final
  version.
- `dry_run`: run every gate and build, push nothing.

The **first release** is dispatched with `bump=major` and `prerelease_id=rc`, which
yields `1.0.0-rc.1` (tag `v1.0.0-rc.1`). Dispatching again with `rc` gives
`1.0.0-rc.2`; an empty `prerelease_id` promotes the candidate to `1.0.0`.

The repository needs these settings:

| Kind | Name | Value |
| --- | --- | --- |
| Variable | `DOCKERHUB_NAMESPACE` | `noixdev` |
| Variable | `DOCKERHUB_IMAGE` | `error-pages` |
| Secret | `DOCKERHUB_USERNAME` | Docker Hub user that may push |
| Secret | `DOCKERHUB_TOKEN` | access token for that user |

The image reads its version from `version.json` at build time, so local builds
(`make build`, `docker build .`) report whatever that file says.

## Reporting bugs and ideas

Use the issue templates. For security problems follow [SECURITY.md](SECURITY.md)
instead. By contributing you agree that your work is licensed under the
[MIT licence](LICENSE).
