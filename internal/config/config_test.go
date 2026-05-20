package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "collections.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValid(t *testing.T) {
	p := write(t, `
[[collection]]
id = "kids-tv"
label = "Kids TV"
source = "/nfs/media/Kids"
dest = "/media/external/Media/Kids"
kind = "tv"

[[collection]]
id = "movies"
label = "Movies"
source = "/nfs/films/Movies"
dest = "/media/external/Media/Movies"
kind = "movie"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Collections) != 2 {
		t.Fatalf("got %d collections, want 2", len(cfg.Collections))
	}
	c, ok := cfg.Collection("movies")
	if !ok || c.Kind != KindMovie || c.Dest != "/media/external/Media/Movies" {
		t.Fatalf("Collection(movies) = %+v, ok=%v", c, ok)
	}
	if _, ok := cfg.Collection("nope"); ok {
		t.Fatal("Collection(nope): want ok=false for an unknown id")
	}
}

func TestLoadRejectsBadKind(t *testing.T) {
	p := write(t, `
[[collection]]
id = "x"
label = "X"
source = "/a"
dest = "/b"
kind = "audiobook"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

func TestLoadRejectsDuplicateID(t *testing.T) {
	p := write(t, `
[[collection]]
id = "x"
label = "X"
source = "/a"
dest = "/b"
kind = "tv"

[[collection]]
id = "x"
label = "Y"
source = "/c"
dest = "/d"
kind = "tv"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for duplicate id")
	}
}

func TestLoadRejectsMissingField(t *testing.T) {
	p := write(t, `
[[collection]]
id = "x"
label = "X"
dest = "/b"
kind = "tv"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for a collection missing its source field")
	}
}
