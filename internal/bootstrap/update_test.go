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
	"slices"
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
		Run: func(name string, args ...string) error {
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
		Out: &bytes.Buffer{},
	}
	if err := u.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if b, _ := os.ReadFile(binPath); string(b) != "new-roadie-binary" {
		t.Errorf("binary not swapped: %q", b)
	}
	if len(calls) != 1 || !slices.Equal(calls[0], []string{"systemctl", "restart", "roadie"}) {
		t.Errorf("expected one `systemctl restart roadie`, got %v", calls)
	}
}

func TestUpdateNoopWhenCurrent(t *testing.T) {
	srv := releaseServer(t, "v1.0.0", []byte("whatever"))
	binPath := filepath.Join(t.TempDir(), "roadie")
	if err := os.WriteFile(binPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
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

func TestUpdateRejectsBadChecksum(t *testing.T) {
	asset := fmt.Sprintf("roadie_%s_%s", runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/bin", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("corrupted-download"))
	})
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		// A checksum for different bytes — it will not match the served binary.
		good := sha256.Sum256([]byte("the-genuine-binary"))
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(good[:]), asset)
	})
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[
			{"name":%q,"browser_download_url":%q},
			{"name":"checksums.txt","browser_download_url":%q}]}`,
			asset, srv.URL+"/bin", srv.URL+"/sums")
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	binPath := filepath.Join(t.TempDir(), "roadie")
	if err := os.WriteFile(binPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	u := &Updater{
		APIURL: srv.URL + "/latest", BinPath: binPath, Current: "v1.0.0",
		Run: func(name string, args ...string) error { return nil },
		Out: &bytes.Buffer{},
	}
	if err := u.Update(); err == nil {
		t.Fatal("Update must return an error when the checksum does not match")
	}
	if b, _ := os.ReadFile(binPath); string(b) != "old-binary" {
		t.Error("a checksum failure must leave the installed binary untouched")
	}
}
