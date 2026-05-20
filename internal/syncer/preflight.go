package syncer

import (
	"fmt"
	"path/filepath"
	"syscall"
)

// IsMountPoint reports whether path is a mount point (its device differs
// from its parent directory's device). Linux-only. Returns false on any stat
// error, and false for the filesystem root "/" (whose parent is itself).
func IsMountPoint(path string) bool {
	var st, parent syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return false
	}
	if err := syscall.Stat(filepath.Dir(path), &parent); err != nil {
		return false
	}
	return st.Dev != parent.Dev
}

// FreeBytes returns the free space, in bytes, of the filesystem at path.
func FreeBytes(path string) (int64, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	return int64(fs.Bavail) * int64(fs.Bsize), nil
}
