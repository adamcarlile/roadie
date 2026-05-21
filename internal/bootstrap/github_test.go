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
	if rel.Assets[0].URL != "http://example/bin" {
		t.Fatalf("asset[0].URL = %q, want http://example/bin", rel.Assets[0].URL)
	}
}

func TestFetchLatestReleaseBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer srv.Close()
	if _, err := FetchLatestRelease(srv.URL); err == nil {
		t.Fatal("expected an error on malformed JSON")
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
