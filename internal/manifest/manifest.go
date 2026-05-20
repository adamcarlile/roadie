// Package manifest owns the roadie desired-state manifest file.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one desired media object: a path within a collection.
type Entry struct {
	Collection string    `json:"collection"`
	Path       string    `json:"path"`
	Added      time.Time `json:"added"`
}

// Manifest is the desired-state document.
type Manifest struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Store owns the manifest file and serialises access to it.
type Store struct {
	path string
	mu   sync.Mutex
	m    Manifest
}

// Open loads the manifest at path. A missing file yields an empty manifest;
// a corrupt file is moved to <path>.corrupt and treated as empty.
func Open(path string) (*Store, error) {
	s := &Store{path: path, m: Manifest{Version: 1}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		// Corrupt manifest: quarantine it (best-effort) and start empty.
		_ = os.Rename(path, path+".corrupt")
		return s, nil
	}
	if m.Version == 0 {
		m.Version = 1
	}
	s.m = m
	return s, nil
}

// Snapshot returns a copy of the current manifest.
func (s *Store) Snapshot() Manifest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Manifest{Version: s.m.Version, Entries: make([]Entry, len(s.m.Entries))}
	copy(out.Entries, s.m.Entries)
	return out
}

// Add inserts an entry. Adding an existing (collection,path) is a no-op.
func (s *Store) Add(collection, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.m.Entries {
		if e.Collection == collection && e.Path == path {
			return nil
		}
	}
	s.m.Entries = append(s.m.Entries, Entry{
		Collection: collection, Path: path, Added: time.Now().UTC(),
	})
	return s.save()
}

// Remove deletes an entry. Removing a missing entry is a no-op.
func (s *Store) Remove(collection, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]Entry, 0, len(s.m.Entries))
	for _, e := range s.m.Entries {
		if e.Collection == collection && e.Path == path {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == len(s.m.Entries) {
		return nil // nothing matched — no write needed
	}
	s.m.Entries = kept
	return s.save()
}

// save writes the manifest atomically (temp file in the same dir + rename).
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".manifest-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, s.path)
}
