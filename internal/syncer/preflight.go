package syncer

import (
	"fmt"
	"path/filepath"
	"syscall"
)

// DriveMounted reports whether dest sits on a drive mounted separately from the
// root filesystem. It walks up to the nearest existing ancestor of dest (dest
// itself need not exist yet — rsync creates it) and compares that path's device
// to "/": a different device means the external drive is mounted. A dest that
// resolves to the root device means the drive is absent, and syncing there
// would fill the system disk. Linux-only.
func DriveMounted(dest string) bool {
	var root syscall.Stat_t
	if syscall.Stat("/", &root) != nil {
		return false
	}
	for p := dest; ; {
		var st syscall.Stat_t
		if syscall.Stat(p, &st) == nil {
			return st.Dev != root.Dev
		}
		parent := filepath.Dir(p)
		if parent == p {
			return false // reached "/" without finding an existing path
		}
		p = parent
	}
}

// FreeBytes returns the free space, in bytes, of the filesystem at path.
func FreeBytes(path string) (int64, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	return int64(fs.Bavail) * int64(fs.Bsize), nil
}
