package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"roadie/internal/config"
	"roadie/internal/manifest"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{Collections: []config.Collection{
		{ID: "movies", Label: "Movies", Source: "/tmp/none", Dest: "/tmp/none", Kind: "movie"},
	}}
	store, err := manifest.Open(filepath.Join(t.TempDir(), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, store, http.NotFoundHandler())
}

func TestCollectionsEndpoint(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/collections", nil)
	testServer(t).Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got []config.Collection
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "movies" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAddAndRemoveEntry(t *testing.T) {
	s := testServer(t)
	add := httptest.NewRecorder()
	body := `{"collection":"movies","path":"Up (2009).mkv"}`
	s.Handler().ServeHTTP(add, httptest.NewRequest("POST", "/api/manifest/entries", strings.NewReader(body)))
	if add.Code != 200 {
		t.Fatalf("add status %d: %s", add.Code, add.Body.String())
	}
	if len(s.store.Snapshot().Entries) != 1 {
		t.Fatal("entry not added")
	}
	del := httptest.NewRecorder()
	s.Handler().ServeHTTP(del, httptest.NewRequest("DELETE",
		"/api/manifest/entries?collection=movies&path=Up+(2009).mkv", nil))
	if del.Code != 200 {
		t.Fatalf("delete status %d", del.Code)
	}
	if len(s.store.Snapshot().Entries) != 0 {
		t.Fatal("entry not removed")
	}
}

func TestSyncRejectsWhenLocked(t *testing.T) {
	s := testServer(t)
	if !s.tryClaimRun() { // a run is already in progress
		t.Fatal("could not claim a run")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/sync", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", rec.Code)
	}
}

func TestSyncStartsWhenIdle(t *testing.T) {
	rec := httptest.NewRecorder()
	testServer(t).Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/sync", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202", rec.Code)
	}
}

func TestAddEntryRejectsMalformedJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	testServer(t).Handler().ServeHTTP(rec, httptest.NewRequest("POST",
		"/api/manifest/entries", strings.NewReader("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestAddEntryRejectsUnknownCollection(t *testing.T) {
	rec := httptest.NewRecorder()
	body := `{"collection":"nope","path":"X.mkv"}`
	testServer(t).Handler().ServeHTTP(rec, httptest.NewRequest("POST",
		"/api/manifest/entries", strings.NewReader(body)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}

func TestRemoveEntryRejectsMissingParams(t *testing.T) {
	rec := httptest.NewRecorder()
	testServer(t).Handler().ServeHTTP(rec, httptest.NewRequest("DELETE",
		"/api/manifest/entries?collection=movies", nil)) // path missing
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestBrowseRejectsUnknownCollection(t *testing.T) {
	rec := httptest.NewRecorder()
	testServer(t).Handler().ServeHTTP(rec, httptest.NewRequest("GET",
		"/api/collections/nope/browse", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}
