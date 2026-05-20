package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMissingStartsEmpty(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "manifest.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := s.Snapshot(); got.Version != 1 || len(got.Entries) != 0 {
		t.Fatalf("snapshot = %+v, want empty v1", got)
	}
}

func TestAddRemoveAndPersist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "manifest.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("movies", "Up (2009).mkv"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("movies", "Up (2009).mkv"); err != nil { // duplicate
		t.Fatal(err)
	}
	if got := s.Snapshot(); len(got.Entries) != 1 {
		t.Fatalf("after dup add: %d entries, want 1", len(got.Entries))
	}
	// Reopen — entry persisted to disk.
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Snapshot().Entries) != 1 {
		t.Fatal("entry did not persist")
	}
	if err := s2.Remove("movies", "Up (2009).mkv"); err != nil {
		t.Fatal(err)
	}
	if len(s2.Snapshot().Entries) != 0 {
		t.Fatal("entry not removed")
	}
	// Reopen once more — the removal persisted to disk.
	s3, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s3.Snapshot().Entries) != 0 {
		t.Fatal("removal did not persist")
	}
}

func TestOpenCorruptMovesAside(t *testing.T) {
	p := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(s.Snapshot().Entries) != 0 {
		t.Fatal("corrupt manifest should open empty")
	}
	if _, err := os.Stat(p + ".corrupt"); err != nil {
		t.Fatal("corrupt file was not moved aside")
	}
}
