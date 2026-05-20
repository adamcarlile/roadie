// Package scan lists pickable media objects on disk and sizes paths.
package scan

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// videoExts are the file extensions treated as media objects.
var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true, ".mov": true,
	".mpg": true, ".mpeg": true, ".ts": true, ".wmv": true,
}

// Object is a pickable item in a collection.
type Object struct {
	Name  string `json:"name"`  // leaf name, used as the path segment
	IsDir bool   `json:"isDir"` // directory (drillable) vs bare file
}

// isJunk reports whether a directory entry should be ignored.
func isJunk(name string) bool {
	// @eaDir is Synology's thumbnail dir; Thumbs.db is the Windows thumbnail cache.
	if name == "@eaDir" || name == "Thumbs.db" {
		return true
	}
	return strings.HasPrefix(name, ".")
}

// Browse lists the pickable objects directly inside dir — subdirectories and
// video files — with junk filtered out, sorted by name.
func Browse(dir string) ([]Object, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var objs []Object
	for _, e := range ents {
		name := e.Name()
		if isJunk(name) {
			continue
		}
		// Symlinks are not followed: a symlinked directory or media file is
		// classified by the link entry itself and skipped. roadie's sources are
		// real directories — this is an intentional v1 simplification.
		if e.IsDir() {
			objs = append(objs, Object{Name: name, IsDir: true})
		} else if videoExts[strings.ToLower(filepath.Ext(name))] {
			objs = append(objs, Object{Name: name, IsDir: false})
		}
	}
	sort.Slice(objs, func(i, j int) bool { return objs[i].Name < objs[j].Name })
	return objs, nil
}

// PathSize returns the total size in bytes of a file or directory tree.
// Any unreadable entry aborts the walk and is returned as an error, so callers
// get a complete total or a failure — never a silent partial.
func PathSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}
