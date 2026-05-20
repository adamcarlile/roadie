# roadie Release Pipeline & Bootstrap — Design Spec

**Date:** 2026-05-20
**Status:** Approved design — ready for implementation planning

---

## Purpose

roadie currently deploys by a manual recipe (`scp` the binary + config + systemd
unit, then `install`/`mkdir`/`cp`/`systemctl` by hand). This design adds:

1. A **GitHub Actions pipeline** that tests roadie on every change and publishes
   versioned binary releases.
2. **Bootstrap subcommands** in the binary — `install`, `update`, `version` — so a
   carnet box can be set up, and kept current, without the manual recipe.
3. A **first-install script** so a bare box goes from nothing to a running service
   with one command.

---

## Context

- roadie is a single Go binary, implemented and on `main` at `github.com/adamcarlile/roadie`.
- The repository is **public** — release fetches need no authentication.
- Releases are cut from **git tags** (`v1.2.3`).
- Target box: the carnet (`linux/amd64`). Releases also cover other targets for
  general use and local development.

---

## The Pipeline

Three version-controlled files.

### `.github/workflows/ci.yml`

Runs on push to `main` and on every pull request — the quality gate, separate
from releasing:

- `gofmt -l` check (fails if any file is unformatted)
- `go vet ./...`
- `go test -race ./...`
- `goreleaser check` (validates `.goreleaser.yaml`)

### `.github/workflows/release.yml`

Runs on push of a `v*` tag:

- Checkout with full history (for the changelog), set up Go.
- `go test ./...` as a release gate.
- Run the GoReleaser action with the repository's `GITHUB_TOKEN`.

### `.goreleaser.yaml`

- **One build, four targets:** `goos: [linux, darwin]` × `goarch: [amd64, arm64]`,
  `CGO_ENABLED=0`.
- **Version injection:** `ldflags: -s -w -X roadie/internal/buildinfo.Version={{.Version}}`.
- **`archives: format: binary`** — publishes the bare per-platform binaries (not
  `.tar.gz`), named on a fixed template (`roadie_{{.Os}}_{{.Arch}}`) so the update
  client can match its own `GOOS/GOARCH` reliably.
- **`checksums.txt`** (sha256) and a git-commit **changelog**, auto-generated.
- **`release`** — publishes a GitHub Release for the tag with binaries + checksums.

Flow: `git tag v1.2.0 && git push --tags` → pipeline tests, builds all four
targets, publishes the Release.

---

## The Bootstrap Command

`main.go` gains a dispatch on the first argument; the logic lives in a new
`internal/bootstrap` package, keeping `main.go` thin and the logic testable.

### Subcommand dispatch (`main.go`)

- `install`, `update`, `version` → the `bootstrap` package.
- Anything else (a flag, or no argument) → **serve** — unchanged current behaviour,
  so the existing `roadie -config … -addr …` invocation still works.

### `internal/buildinfo`

A one-line package: `var Version = "dev"`. GoReleaser's ldflags set it at build
time. `roadie version` prints it; `roadie update` compares against it. Both `main`
and `bootstrap` import it (a `package main` variable could not be shared).

### `roadie install` — guided confirm-then-install

- Requires root (`os.Geteuid() == 0`, else a clear "re-run with sudo").
- Prints exactly what it will do:
  - copy the running executable (`os.Executable()`) → `/usr/local/bin/roadie`
  - create `/etc/roadie/` and write `collections.toml` **only if absent** (never
    clobber an existing config)
  - create `/var/lib/roadie/`
  - write `/etc/systemd/system/roadie.service` (generated to run `roadie serve`)
  - `systemctl daemon-reload`, then `systemctl enable --now roadie`
- Prompts `Proceed? [y/N]`; on `y` performs the steps and reports, otherwise aborts.
- The default config is `collections.toml.example`, **embedded** into the binary
  via `//go:embed`.

### `roadie update` — self-update

- Requires root.
- Queries `https://api.github.com/repos/adamcarlile/roadie/releases/latest`
  (anonymous; public repo) for the latest `tag_name` and assets.
- If the latest tag equals `buildinfo.Version` (normalising a leading `v`) →
  reports "already current" and exits.
- Otherwise: downloads the asset matching `runtime.GOOS_runtime.GOARCH`, verifies
  its sha256 against the release's `checksums.txt`, atomically replaces
  `/usr/local/bin/roadie` (temp file + rename on the same filesystem), then
  `systemctl restart roadie`. Reports the old → new version.

### GitHub releases client

A small piece of `internal/bootstrap` using stdlib `net/http` + `encoding/json`
(no dependency): fetch the latest release, expose its tag and assets.

### Testability

`internal/bootstrap` is structured for tests: an `Installer` struct carries the
target paths (`/usr/local/bin`, `/etc/roadie`, `/var/lib/roadie`, the unit dir)
and a command-runner func — both defaulted to the real system, overridable in
tests. `Install()` can then run against a `t.TempDir()` with a fake runner that
records the `systemctl` invocations.

---

## The Install Script

`install.sh` at the repository root. One-liner:

```
curl -sSL https://raw.githubusercontent.com/adamcarlile/roadie/main/install.sh | sudo sh
```

POSIX `sh`, ~30 lines:

- Detect platform (`uname -s` / `uname -m` → `linux_amd64`, etc.).
- Download the binary and `checksums.txt` from GitHub's stable
  `https://github.com/adamcarlile/roadie/releases/latest/download/<asset>`
  redirect — that URL always points at the newest release, so no API call or
  JSON parsing is needed in shell.
- Verify the sha256 (`sha256sum -c`), `chmod +x`, then `exec` the downloaded
  binary's `install` subcommand — handing off to the guided confirm-then-install.

A bare carnet box thus goes from nothing to a running, enabled service in one
command. (`roadie update` uses the GitHub API rather than this redirect because
it needs the version string to compare; the script only needs the bytes.)

---

## Testing

- **Unit tests** (`internal/bootstrap`): the releases-JSON parser; asset selection
  by `GOOS/GOARCH`; sha256 verification against a `checksums.txt`; the `Installer`
  orchestration against a temp dir with a fake command runner.
- **Pipeline:** `goreleaser check` validates `.goreleaser.yaml` in CI;
  `goreleaser release --snapshot --clean` is the local dry-run that builds all
  four targets without publishing. The workflows themselves are verified by first
  real use (push a branch → CI; push a tag → release).
- **Install script:** verified by running it; optionally `shellcheck`ed in CI.

---

## v1 Scope & Deferred Work

**v1** is everything above: the CI + release workflows, GoReleaser config, the
`install` / `update` / `version` subcommands, and the install script.

**Deferred** (noted, not built):

- **Scheduled auto-update** — a systemd timer running `roadie update`. For now
  update is operator-invoked.
- **Update rollback** — a bad binary is recovered by re-running `roadie update`
  or the install one-liner; an automatic rollback is future work.

---

## Key Decisions

| Decision | Choice | Reasoning |
|----------|--------|-----------|
| Release tooling | **GoReleaser** | The standard Go release tool; multi-platform artifacts and checksums are genuinely useful and more complete than a hand-rolled single-target workflow. |
| Archive format | **`format: binary`** (bare binaries) | Keeps `roadie update` a download-and-swap with no extraction step. |
| Release trigger | **Git tags (`v*`)** | The operator controls release timing; the tag is the version, giving `roadie update` meaningful versions to compare. |
| Repo visibility | **Public** | No token to manage on the carnet; the install script and `update` fetch anonymously. |
| Bootstrap scope | **Full lifecycle** | `install` + `update` in the binary, plus a first-install script — a bare box can be brought fully up and kept current. |
| Guided setup depth | **Confirm defaults** | Show the standard config + install plan, confirm, install. Tailoring is hand-editing the TOML afterwards — minimal surface for v1. |
| Version sharing | **`internal/buildinfo` package** | A `package main` variable cannot be imported by `internal/bootstrap`; a tiny shared package can. |
