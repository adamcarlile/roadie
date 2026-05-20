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
		os.Remove(tmp)
		return fmt.Errorf("writing new binary: %w", err)
	}
	if err := os.Chmod(tmp, 0o755); err != nil { // umask-proof the executable bit
		os.Remove(tmp)
		return fmt.Errorf("setting binary mode: %w", err)
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

// maxDownloadBytes caps a release download — a generous bound for a binary, so
// a runaway or malicious response cannot exhaust memory before the checksum
// can reject it.
const maxDownloadBytes = 100 << 20 // 100 MiB

// download fetches url and returns the response body, capped at maxDownloadBytes.
func download(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes))
}
