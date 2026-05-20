package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseListsDirsAndVideosSkippingJunk(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "Cars (2006)"))
	mustMkdir(t, filepath.Join(dir, "@eaDir"))
	mustWrite(t, filepath.Join(dir, "Up (2009).mkv"), "video")
	mustWrite(t, filepath.Join(dir, ".DS_Store"), "junk")
	mustWrite(t, filepath.Join(dir, "._Up (2009).mkv"), "junk")
	mustWrite(t, filepath.Join(dir, "notes.txt"), "not video")

	objs, err := Browse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 {
		t.Fatalf("got %d objects, want 2: %+v", len(objs), objs)
	}
	if objs[0].Name != "Cars (2006)" || !objs[0].IsDir {
		t.Fatalf("objs[0] = %+v", objs[0])
	}
	if objs[1].Name != "Up (2009).mkv" || objs[1].IsDir {
		t.Fatalf("objs[1] = %+v", objs[1])
	}
}

func TestPathSizeSumsTree(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.mkv"), "12345")
	sub := filepath.Join(dir, "sub")
	mustMkdir(t, sub)
	mustWrite(t, filepath.Join(sub, "b.mkv"), "678")
	got, err := PathSize(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != 8 { // "12345" (5 bytes) + "678" (3 bytes)
		t.Fatalf("PathSize = %d, want 8", got)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
