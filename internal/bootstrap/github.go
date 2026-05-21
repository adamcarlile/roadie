// Package bootstrap installs roadie as a systemd service and self-updates it
// from GitHub releases.
package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpClient is used for all GitHub API and release-download requests. Its
// timeout stops a hung connection from blocking the update path indefinitely.
var httpClient = &http.Client{Timeout: 30 * time.Second}

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
	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return Release{}, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Release{}, fmt.Errorf("fetching latest release: HTTP %d: %s", resp.StatusCode, body)
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
// GoReleaser asset name "roadie_<os>_<arch>".
func (r Release) AssetFor(goos, goarch string) (Asset, bool) {
	return r.Asset(fmt.Sprintf("roadie_%s_%s", goos, goarch))
}
