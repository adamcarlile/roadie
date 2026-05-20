# roadie Release Pipeline & Bootstrap — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give roadie a GitHub Actions release pipeline (GoReleaser, tag-triggered) and `install` / `update` / `version` subcommands so a carnet box can be set up and kept current without the manual deploy recipe.

**Architecture:** A `buildinfo` package holds the ldflags-injected version. A `bootstrap` package holds a stdlib GitHub-releases client, sha256 verification, a guided `Installer`, and a self-`Updater` — paths and command execution are struct fields so the system-mutating code is testable against temp dirs. `main.go` dispatches subcommands; `serve` is the unchanged default. GoReleaser builds four targets and publishes bare per-platform binaries; an `install.sh` one-liner fetches and runs the guided install.

**Tech Stack:** Go 1.23 (stdlib `net/http`, `crypto/sha256`, `embed`), GoReleaser, GitHub Actions, POSIX `sh`.

**Prerequisites:** Go 1.23+ and `git` available. GoReleaser is needed only for the Task 7 local verification (`go install github.com/goreleaser/goreleaser/v2@latest`, or skip — CI validates it); `shellcheck` is optional for Task 9. Reference spec: `docs/superpowers/specs/2026-05-20-release-and-bootstrap-design.md`.

**Conventions for every task:** run `gofmt -w` on changed Go files; `go vet ./...` and `go test ./...` must pass before each commit. Commit messages have a clear imperative subject — and **never** a `Co-Authored-By` trailer or any AI-attribution line.

---

### Task 1: buildinfo package

Holds the version string, set by the linker at release build time.

**Files:**
- Create: `internal/buildinfo/buildinfo.go`

- [ ] **Step 1: Create the package**

`internal/buildinfo/buildinfo.go`:
```go
// Package buildinfo carries build-time version information.
package buildinfo

// Version is the roadie version. It is "dev" for local builds and is set to
// the release tag at build time via -ldflags "-X .../buildinfo.Version=...".
var Version = "dev"
```

- [ ] **Step 2: Verify it builds**

Run: `go -C /home/adam/roadie build ./...`
Expected: builds clean, no output.

- [ ] **Step 3: Commit**

```bash
gofmt -w internal/buildinfo/
git add internal/buildinfo/
git commit -m "Add buildinfo package for the version string"
```

---

### Task 2: GitHub releases client

Fetches the latest release from the GitHub API and selects a platform asset.

**Files:**
- Create: `internal/bootstrap/github.go`
- Test: `internal/bootstrap/github_test.go`

- [ ] **Step 1: Write the failing test**

`internal/bootstrap/github_test.go`:
```go
package bootstrap

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v1.4.0","assets":[
			{"name":"roadie_linux_amd64","browser_download_url":"http://example/bin"},
			{"name":"checksums.txt","browser_download_url":"http://example/sums"}]}`)
	}))
	defer srv.Close()

	rel, err := FetchLatestRelease(srv.URL)
	if err != nil {
		t.Fatalf("FetchLatestRelease: %v", err)
	}
	if rel.Tag != "v1.4.0" {
		t.Fatalf("tag = %q, want v1.4.0", rel.Tag)
	}
	if len(rel.Assets) != 2 {
		t.Fatalf("got %d assets, want 2", len(rel.Assets))
	}
}

func TestFetchLatestReleaseHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := FetchLatestRelease(srv.URL); err == nil {
		t.Fatal("expected an error on HTTP 404")
	}
}

func TestAssetForAndAsset(t *testing.T) {
	rel := Release{Assets: []Asset{
		{Name: "roadie_linux_amd64", URL: "u1"},
		{Name: "roadie_darwin_arm64", URL: "u2"},
		{Name: "checksums.txt", URL: "u3"},
	}}
	a, ok := rel.AssetFor("linux", "amd64")
	if !ok || a.URL != "u1" {
		t.Fatalf("AssetFor(linux,amd64) = %+v, %v", a, ok)
	}
	if _, ok := rel.AssetFor("windows", "amd64"); ok {
		t.Error("AssetFor(windows,amd64) should report not found")
	}
	if c, ok := rel.Asset("checksums.txt"); !ok || c.URL != "u3" {
		t.Fatalf("Asset(checksums.txt) = %+v, %v", c, ok)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go -C /home/adam/roadie test ./internal/bootstrap/`
Expected: FAIL — `undefined: FetchLatestRelease` (package does not compile).

- [ ] **Step 3: Write the implementation**

`internal/bootstrap/github.go`:
```go
// Package bootstrap installs roadie as a systemd service and self-updates it
// from GitHub releases.
package bootstrap

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Asset is one downloadable file attached to a GitHub release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Release is a published GitHub release.
type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

// FetchLatestRelease retrieves the latest release from a GitHub releases API
// URL, e.g. https://api.github.com/repos/adamcarlile/roadie/releases/latest.
func FetchLatestRelease(apiURL string) (Release, error) {
	resp, err := http.Get(apiURL)
	if err != nil {
		return Release{}, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("fetching latest release: HTTP %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("decoding release: %w", err)
	}
	return rel, nil
}

// Asset returns the release asset with the exact given name.
func (r Release) Asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// AssetFor returns the binary asset for the given GOOS/GOARCH, matching the
// GoReleaser archive name "roadie_<os>_<arch>".
func (r Release) AssetFor(goos, goarch string) (Asset, bool) {
	return r.Asset(fmt.Sprintf("roadie_%s_%s", goos, goarch))
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go -C /home/adam/roadie test ./internal/bootstrap/`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/bootstrap/
git add internal/bootstrap/
git commit -m "Add GitHub releases client"
```

---

### Task 3: Checksum verification

Verifies a downloaded binary against a GoReleaser `checksums.txt`.

**Files:**
- Create: `internal/bootstrap/checksum.go`
- Test: `internal/bootstrap/checksum_test.go`

- [ ] **Step 1: Write the failing test**

`internal/bootstrap/checksum_test.go`:
```go
package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifySHA256(t *testing.T) {
	data := []byte("the roadie binary")
	sum := sha256.Sum256(data)
	checksums := hex.EncodeToString(sum[:]) + "  roadie_linux_amd64\n" +
		"0000000000000000000000000000000000000000000000000000000000000000  other\n"

	if err := VerifySHA256(data, checksums, "roadie_linux_amd64"); err != nil {
		t.Fatalf("VerifySHA256 on matching data: %v", err)
	}
	if err := VerifySHA256([]byte("tampered"), checksums, "roadie_linux_amd64"); err == nil {
		t.Error("expected a mismatch error for tampered data")
	}
	if err := VerifySHA256(data, checksums, "roadie_linux_arm64"); err == nil {
		t.Error("expected an error when the asset is absent from checksums.txt")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go -C /home/adam/roadie test ./internal/bootstrap/ -run TestVerifySHA256`
Expected: FAIL — `undefined: VerifySHA256`.

- [ ] **Step 3: Write the implementation**

`internal/bootstrap/checksum.go`:
```go
package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// VerifySHA256 checks that data's SHA-256 digest matches the entry for
// assetName in a GoReleaser checksums.txt body (lines of "<hex>  <filename>").
// It returns an error if assetName is not listed or the digest does not match.
func VerifySHA256(data []byte, checksumsTxt, assetName string) error {
	want := ""
	for _, line := range strings.Split(checksumsTxt, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum listed for %s", assetName)
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go -C /home/adam/roadie test ./internal/bootstrap/ -run TestVerifySHA256`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/bootstrap/
git add internal/bootstrap/
git commit -m "Add release checksum verification"
```

---

### Task 4: The Installer

The guided confirm-then-install. Paths and the command runner are struct fields, so the test runs it against a temp directory with a fake runner.

**Files:**
- Create: `internal/bootstrap/install.go`
- Test: `internal/bootstrap/install_test.go`

- [ ] **Step 1: Write the failing test**

`internal/bootstrap/install_test.go`:
```go
package bootstrap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempInstaller builds an Installer rooted entirely in a temp directory, with a
// fake command runner that records calls. confirmInput is fed to the prompt.
func tempInstaller(t *testing.T, confirmInput string) (*Installer, *[][]string) {
	t.Helper()
	root := t.TempDir()
	var calls [][]string
	in := &Installer{
		BinDir:        filepath.Join(root, "bin"),
		ConfigDir:     filepath.Join(root, "etc"),
		StateDir:      filepath.Join(root, "state"),
		UnitDir:       filepath.Join(root, "units"),
		DefaultConfig: "# default config\n",
		UnitFile:      "[Unit]\n",
		Run:           func(name string, args ...string) error { calls = append(calls, append([]string{name}, args...)); return nil },
		In:            strings.NewReader(confirmInput),
		Out:           &bytes.Buffer{},
	}
	return in, &calls
}

func fakeExe(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "roadie")
	if err := os.WriteFile(p, []byte("ELF-ish binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInstallCreatesEverything(t *testing.T) {
	in, calls := tempInstaller(t, "y\n")
	if err := in.Install(fakeExe(t)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(in.BinDir, "roadie")); err != nil || string(b) != "ELF-ish binary" {
		t.Errorf("binary not installed: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(in.ConfigDir, "collections.toml")); err != nil || string(b) != "# default config\n" {
		t.Errorf("config not written: %v", err)
	}
	if _, err := os.Stat(in.StateDir); err != nil {
		t.Errorf("state dir not created: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(in.UnitDir, "roadie.service")); err != nil || string(b) != "[Unit]\n" {
		t.Errorf("unit not written: %v", err)
	}
	if len(*calls) != 2 || (*calls)[0][1] != "daemon-reload" || (*calls)[1][2] != "--now" {
		t.Errorf("systemctl calls = %v", *calls)
	}
}

func TestInstallDeclined(t *testing.T) {
	in, calls := tempInstaller(t, "n\n")
	if err := in.Install(fakeExe(t)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(in.BinDir, "roadie")); !os.IsNotExist(err) {
		t.Error("declining should install nothing")
	}
	if len(*calls) != 0 {
		t.Errorf("declining should run no commands, got %v", *calls)
	}
}

func TestInstallKeepsExistingConfig(t *testing.T) {
	in, _ := tempInstaller(t, "y\n")
	if err := os.MkdirAll(in.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(in.ConfigDir, "collections.toml")
	if err := os.WriteFile(existing, []byte("# my edited config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := in.Install(fakeExe(t)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if b, _ := os.ReadFile(existing); string(b) != "# my edited config\n" {
		t.Error("Install must not clobber an existing config")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go -C /home/adam/roadie test ./internal/bootstrap/ -run TestInstall`
Expected: FAIL — `undefined: Installer`.

- [ ] **Step 3: Write the implementation**

`internal/bootstrap/install.go`:
```go
package bootstrap

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Installer sets roadie up as a systemd service. Its paths and command runner
// are fields so tests can target a temp directory and a fake runner.
type Installer struct {
	BinDir        string // e.g. /usr/local/bin
	ConfigDir     string // e.g. /etc/roadie
	StateDir      string // e.g. /var/lib/roadie
	UnitDir       string // e.g. /etc/systemd/system
	DefaultConfig string // collections.toml contents, written when none exists
	UnitFile      string // systemd unit file contents
	Run           func(name string, args ...string) error
	In            io.Reader // confirmation-prompt input
	Out           io.Writer // prompt and report output
}

// DefaultInstaller returns an Installer targeting the real system paths.
func DefaultInstaller(defaultConfig, unitFile string) *Installer {
	return &Installer{
		BinDir:        "/usr/local/bin",
		ConfigDir:     "/etc/roadie",
		StateDir:      "/var/lib/roadie",
		UnitDir:       "/etc/systemd/system",
		DefaultConfig: defaultConfig,
		UnitFile:      unitFile,
		Run:           func(name string, args ...string) error { return exec.Command(name, args...).Run() },
		In:            os.Stdin,
		Out:           os.Stdout,
	}
}

// Install copies the executable at exePath into place, writes the config (only
// if absent), state directory and systemd unit, and enables the service —
// after printing the plan and getting a y/N confirmation.
func (in *Installer) Install(exePath string) error {
	binPath := filepath.Join(in.BinDir, "roadie")
	configPath := filepath.Join(in.ConfigDir, "collections.toml")
	unitPath := filepath.Join(in.UnitDir, "roadie.service")

	fmt.Fprintln(in.Out, "roadie install will:")
	fmt.Fprintf(in.Out, "  - install the binary to %s\n", binPath)
	fmt.Fprintf(in.Out, "  - create %s and write %s (if absent)\n", in.ConfigDir, configPath)
	fmt.Fprintf(in.Out, "  - create %s\n", in.StateDir)
	fmt.Fprintf(in.Out, "  - write %s and enable the service\n", unitPath)
	fmt.Fprint(in.Out, "Proceed? [y/N] ")
	if !confirm(in.In) {
		fmt.Fprintln(in.Out, "aborted.")
		return nil
	}

	for _, dir := range []string{in.BinDir, in.ConfigDir, in.StateDir, in.UnitDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	if err := copyFile(exePath, binPath, 0o755); err != nil {
		return fmt.Errorf("installing binary: %w", err)
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(in.DefaultConfig), 0o644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
	}
	if err := os.WriteFile(unitPath, []byte(in.UnitFile), 0o644); err != nil {
		return fmt.Errorf("writing unit: %w", err)
	}
	if err := in.Run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if err := in.Run("systemctl", "enable", "--now", "roadie"); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}
	fmt.Fprintln(in.Out, "roadie installed and started.")
	return nil
}

// confirm reads one line and reports whether it is an affirmative y / yes.
func confirm(r io.Reader) bool {
	line, _ := bufio.NewReader(r).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// copyFile copies src to dst with the given mode via a temp file + rename.
func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go -C /home/adam/roadie test ./internal/bootstrap/ -run TestInstall`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/bootstrap/
git add internal/bootstrap/
git commit -m "Add guided installer"
```

---

### Task 5: The Updater

Self-update: fetch the latest release, verify the checksum, swap the binary, restart the service.

**Files:**
- Create: `internal/bootstrap/update.go`
- Test: `internal/bootstrap/update_test.go`

- [ ] **Step 1: Write the failing test**

`internal/bootstrap/update_test.go`:
```go
package bootstrap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// releaseServer serves a fake "latest release" plus its binary and checksums.
func releaseServer(t *testing.T, tag string, binData []byte) *httptest.Server {
	t.Helper()
	asset := fmt.Sprintf("roadie_%s_%s", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(binData)
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/bin", func(w http.ResponseWriter, r *http.Request) { w.Write(binData) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
	})
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[
			{"name":%q,"browser_download_url":%q},
			{"name":"checksums.txt","browser_download_url":%q}]}`,
			tag, asset, srv.URL+"/bin", srv.URL+"/sums")
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateSwapsBinaryAndRestarts(t *testing.T) {
	srv := releaseServer(t, "v2.0.0", []byte("new-roadie-binary"))
	binPath := filepath.Join(t.TempDir(), "roadie")
	if err := os.WriteFile(binPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	u := &Updater{
		APIURL: srv.URL + "/latest", BinPath: binPath, Current: "v1.0.0",
		Run: func(name string, args ...string) error { calls = append(calls, append([]string{name}, args...)); return nil },
		Out: &bytes.Buffer{},
	}
	if err := u.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if b, _ := os.ReadFile(binPath); string(b) != "new-roadie-binary" {
		t.Errorf("binary not swapped: %q", b)
	}
	if len(calls) != 1 || calls[0][1] != "restart" {
		t.Errorf("expected one `systemctl restart`, got %v", calls)
	}
}

func TestUpdateNoopWhenCurrent(t *testing.T) {
	srv := releaseServer(t, "v1.0.0", []byte("whatever"))
	binPath := filepath.Join(t.TempDir(), "roadie")
	os.WriteFile(binPath, []byte("old-binary"), 0o755)
	var calls [][]string
	u := &Updater{
		APIURL: srv.URL + "/latest", BinPath: binPath, Current: "v1.0.0",
		Run: func(name string, args ...string) error { calls = append(calls, []string{name}); return nil },
		Out: &bytes.Buffer{},
	}
	if err := u.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if b, _ := os.ReadFile(binPath); string(b) != "old-binary" {
		t.Error("an up-to-date roadie should not swap the binary")
	}
	if len(calls) != 0 {
		t.Errorf("an up-to-date roadie should not restart, got %v", calls)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go -C /home/adam/roadie test ./internal/bootstrap/ -run TestUpdate`
Expected: FAIL — `undefined: Updater`.

- [ ] **Step 3: Write the implementation**

`internal/bootstrap/update.go`:
```go
package bootstrap

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Updater self-updates the installed roadie binary from GitHub releases.
type Updater struct {
	APIURL  string // GitHub "releases/latest" API URL
	BinPath string // path of the installed binary to replace
	Current string // currently-running version (buildinfo.Version)
	Run     func(name string, args ...string) error
	Out     io.Writer
}

// DefaultUpdater returns an Updater for the real system.
func DefaultUpdater(current string) *Updater {
	return &Updater{
		APIURL:  "https://api.github.com/repos/adamcarlile/roadie/releases/latest",
		BinPath: "/usr/local/bin/roadie",
		Current: current,
		Run:     func(name string, args ...string) error { return exec.Command(name, args...).Run() },
		Out:     os.Stdout,
	}
}

// Update fetches the latest release; if it is newer than Current it downloads
// the matching asset, verifies its checksum, atomically swaps the binary, and
// restarts the roadie service.
func (u *Updater) Update() error {
	rel, err := FetchLatestRelease(u.APIURL)
	if err != nil {
		return err
	}
	if sameVersion(rel.Tag, u.Current) {
		fmt.Fprintf(u.Out, "already up to date (%s)\n", u.Current)
		return nil
	}
	bin, ok := rel.AssetFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return fmt.Errorf("release %s has no binary for %s/%s", rel.Tag, runtime.GOOS, runtime.GOARCH)
	}
	sums, ok := rel.Asset("checksums.txt")
	if !ok {
		return fmt.Errorf("release %s has no checksums.txt", rel.Tag)
	}
	binData, err := download(bin.URL)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", bin.Name, err)
	}
	sumData, err := download(sums.URL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	if err := VerifySHA256(binData, string(sumData), bin.Name); err != nil {
		return err
	}
	tmp := u.BinPath + ".new"
	if err := os.WriteFile(tmp, binData, 0o755); err != nil {
		return fmt.Errorf("writing new binary: %w", err)
	}
	if err := os.Rename(tmp, u.BinPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("swapping binary: %w", err)
	}
	if err := u.Run("systemctl", "restart", "roadie"); err != nil {
		return fmt.Errorf("restarting roadie: %w", err)
	}
	fmt.Fprintf(u.Out, "updated %s -> %s\n", u.Current, rel.Tag)
	return nil
}

// sameVersion compares two version strings, ignoring a leading "v".
func sameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// download fetches url and returns the response body.
func download(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go -C /home/adam/roadie test ./internal/bootstrap/`
Expected: PASS (all bootstrap tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/bootstrap/
git add internal/bootstrap/
git commit -m "Add self-update"
```

---

### Task 6: Subcommand dispatch in main.go

Wire `install` / `update` / `version` into the binary; `serve` stays the default. Also update the systemd unit to invoke `roadie serve`.

**Files:**
- Modify: `main.go` (full rewrite)
- Modify: `deploy/roadie.service`

- [ ] **Step 1: Update the systemd unit to use the `serve` subcommand**

In `deploy/roadie.service`, change the `ExecStart` line from:
```
ExecStart=/usr/local/bin/roadie -config /etc/roadie/collections.toml -manifest /var/lib/roadie/manifest.json -addr 0.0.0.0:8473
```
to:
```
ExecStart=/usr/local/bin/roadie serve -config /etc/roadie/collections.toml -manifest /var/lib/roadie/manifest.json -addr 0.0.0.0:8473
```

- [ ] **Step 2: Rewrite `main.go`**

`main.go`:
```go
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"roadie/internal/bootstrap"
	"roadie/internal/buildinfo"
	"roadie/internal/config"
	"roadie/internal/manifest"
	"roadie/internal/server"
)

//go:embed collections.toml.example
var defaultConfig string

//go:embed deploy/roadie.service
var unitFile string

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "version":
			fmt.Println(buildinfo.Version)
			return
		case "install":
			runInstall()
			return
		case "update":
			runUpdate()
			return
		case "serve":
			args = args[1:] // serve consumes the rest as flags
		}
	}
	serve(args)
}

// runInstall installs roadie as a service (requires root).
func runInstall() {
	if os.Geteuid() != 0 {
		log.Fatal("roadie install must be run as root (try: sudo roadie install)")
	}
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("locating executable: %v", err)
	}
	if err := bootstrap.DefaultInstaller(defaultConfig, unitFile).Install(exe); err != nil {
		log.Fatalf("install: %v", err)
	}
}

// runUpdate updates the installed roadie binary (requires root).
func runUpdate() {
	if os.Geteuid() != 0 {
		log.Fatal("roadie update must be run as root (try: sudo roadie update)")
	}
	if err := bootstrap.DefaultUpdater(buildinfo.Version).Update(); err != nil {
		log.Fatalf("update: %v", err)
	}
}

// serve runs the roadie HTTP server, parsing args as flags.
func serve(args []string) {
	fs := flag.NewFlagSet("roadie", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/roadie/collections.toml", "path to collections.toml")
	manPath := fs.String("manifest", "/var/lib/roadie/manifest.json", "path to manifest.json")
	addr := fs.String("addr", "0.0.0.0:8473", "listen address")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("flags: %v", err)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	store, err := manifest.Open(*manPath)
	if err != nil {
		log.Fatalf("manifest: %v", err)
	}
	srv := server.New(cfg, store, server.WebHandler())
	log.Printf("roadie %s listening on %s", buildinfo.Version, *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
```

- [ ] **Step 3: Verify build, vet, and the full test suite**

Run: `gofmt -w main.go && go -C /home/adam/roadie vet ./... && go -C /home/adam/roadie test ./...`
Expected: all clean, all tests pass.

- [ ] **Step 4: Verify the subcommands end to end**

Run:
```bash
go -C /home/adam/roadie build -o /tmp/roadie-check .
/tmp/roadie-check version
/tmp/roadie-check install   # expect: fatal "must be run as root" (run as non-root)
```
Expected: `version` prints `dev`; `install` exits with the root-required message (proving dispatch works without performing a real install).

- [ ] **Step 5: Commit**

```bash
git add main.go deploy/roadie.service
git commit -m "Add install/update/version subcommands"
```

---

### Task 7: GoReleaser configuration

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Create `.goreleaser.yaml`**

```yaml
version: 2

builds:
  - id: roadie
    main: .
    binary: roadie
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w -X roadie/internal/buildinfo.Version={{ .Version }}

archives:
  - id: roadie
    format: binary
    name_template: "roadie_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

changelog:
  use: git

release:
  github:
    owner: adamcarlile
    name: roadie
```

- [ ] **Step 2: Validate the config**

Run: `cd /home/adam/roadie && goreleaser check`
Expected: `1 configuration file(s) validated` (no errors).
If `goreleaser` is not installed: `go install github.com/goreleaser/goreleaser/v2@latest` first, or skip this step — CI (Task 8) validates it on first push.

- [ ] **Step 3: Dry-run the release build**

Run: `cd /home/adam/roadie && goreleaser release --snapshot --clean`
Expected: builds succeed; `dist/` contains `roadie_linux_amd64`, `roadie_linux_arm64`, `roadie_darwin_amd64`, `roadie_darwin_arm64`, and `checksums.txt`. (Skip if `goreleaser` is unavailable.)

- [ ] **Step 4: Commit**

```bash
echo "/dist/" >> /home/adam/roadie/.gitignore
git add .goreleaser.yaml .gitignore
git commit -m "Add GoReleaser configuration"
```

---

### Task 8: GitHub Actions workflows

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create the CI workflow**

`.github/workflows/ci.yml`:
```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: gofmt
        run: |
          unformatted=$(gofmt -l .)
          if [ -n "$unformatted" ]; then
            echo "unformatted files:"; echo "$unformatted"; exit 1
          fi
      - name: vet
        run: go vet ./...
      - name: test
        run: go test -race ./...
      - name: goreleaser check
        uses: goreleaser/goreleaser-action@v6
        with:
          args: check
```

- [ ] **Step 2: Create the release workflow**

`.github/workflows/release.yml`:
```yaml
name: Release

on:
  push:
    tags: ['v*']

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: test
        run: go test ./...
      - name: release
        uses: goreleaser/goreleaser-action@v6
        with:
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/
git commit -m "Add CI and release workflows"
```

Note: the workflows cannot be unit-tested. They are verified by first real use — pushing a branch/PR triggers CI; pushing a `v*` tag triggers the release. This happens during the rollout after the plan is merged.

---

### Task 9: First-install script

**Files:**
- Create: `install.sh`

- [ ] **Step 1: Create `install.sh`**

```sh
#!/bin/sh
# roadie installer — downloads the latest release and runs the guided setup.
# Usage: curl -sSL https://raw.githubusercontent.com/adamcarlile/roadie/main/install.sh | sudo sh
set -e

REPO="adamcarlile/roadie"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "roadie: unsupported architecture: $arch" >&2; exit 1 ;;
esac
asset="roadie_${os}_${arch}"
base="https://github.com/${REPO}/releases/latest/download"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${asset}..."
curl -fsSL -o "$tmp/roadie" "${base}/${asset}"
curl -fsSL -o "$tmp/checksums.txt" "${base}/checksums.txt"

echo "Verifying checksum..."
( cd "$tmp" && grep " ${asset}$" checksums.txt | sed "s/${asset}/roadie/" | sha256sum -c - )

chmod +x "$tmp/roadie"
echo "Starting guided setup..."
# Read the confirmation prompt from the real terminal, since stdin here is the
# piped install script, not the tty. Do not exec, so the trap cleans up $tmp.
"$tmp/roadie" install < /dev/tty
```

- [ ] **Step 2: Make it executable and lint it**

Run:
```bash
chmod +x /home/adam/roadie/install.sh
shellcheck /home/adam/roadie/install.sh
```
Expected: `shellcheck` reports no issues. If `shellcheck` is not installed, skip — it is advisory.

- [ ] **Step 3: Commit**

```bash
git add install.sh
git commit -m "Add first-install script"
```

Note: `install.sh` cannot be fully exercised until a release exists. It is verified for real during rollout, by running the one-liner against the first published release.

---

### Task 10: Update the README

Replace the manual deploy recipe with the install one-liner, `roadie update`, and `roadie version`.

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace the "Build & deploy" section**

In `README.md`, replace the entire `## Build & deploy` section (the `GOOS=linux ... go build`, `scp`, and on-device `install`/`mkdir`/`cp`/`systemctl` recipe) with:

```markdown
## Install

On the carnet box, one command:

```bash
curl -sSL https://raw.githubusercontent.com/adamcarlile/roadie/main/install.sh | sudo sh
```

It downloads the latest release, verifies its checksum, and runs a guided setup
that installs the binary, writes a default config, and enables the service. The
UI is then at `http://carnet.home.adamcarlile.com:8473`.

## Update

```bash
sudo roadie update
```

Fetches the latest release, verifies it, swaps the binary, and restarts the
service. `roadie version` prints the running version.

## Releasing

Releases are cut from git tags. Pushing a tag builds and publishes a GitHub
Release:

```bash
git tag v1.2.0
git push origin v1.2.0
```

## Build from source

```bash
go build -o roadie .
```
```

- [ ] **Step 2: Verify the build is unaffected and commit**

Run: `go -C /home/adam/roadie build ./...`
Expected: clean.

```bash
git add README.md
git commit -m "Document install, update, and releasing"
```

---

## Self-Review Notes

- **Spec coverage:** CI workflow (Task 8), release workflow (Task 8), `.goreleaser.yaml` with four targets + version injection + bare-binary archives + checksums (Task 7), `internal/buildinfo` (Task 1), subcommand dispatch (Task 6), `install` guided confirm-then-install (Task 4), `update` self-update with checksum verification (Task 5), the GitHub releases client (Task 2), the install script (Task 9), README (Task 10) — all covered.
- **Type consistency:** `bootstrap.Release`/`Asset`, `FetchLatestRelease`, `(Release).Asset`/`AssetFor`, `VerifySHA256`, `Installer`/`DefaultInstaller`/`Install`, `Updater`/`DefaultUpdater`/`Update`, `buildinfo.Version` are used consistently across tasks. The GoReleaser archive name `roadie_{{.Os}}_{{.Arch}}` (Task 7) matches `AssetFor`'s `roadie_<goos>_<goarch>` (Task 2) and the install script's `roadie_${os}_${arch}` (Task 9). The ldflags path `roadie/internal/buildinfo.Version` (Task 7) matches the package from Task 1.
- **Untestable-by-unit items:** the workflows (Task 8) and `install.sh` (Task 9) are verified by first real use during rollout, not by unit tests — noted in those tasks. `main.go` (Task 6) is verified by an end-to-end subcommand check.
