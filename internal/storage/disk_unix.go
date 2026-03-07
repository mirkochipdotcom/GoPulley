//go:build !windows

package storage

import "golang.org/x/sys/unix"

// GetDiskUsage returns free and total disk space in bytes for the specified path.
func GetDiskUsage(path string) (free, total int64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	// Bavail is the free blocks available to unprivileged user
	free = int64(stat.Bavail) * int64(stat.Bsize)
	total = int64(stat.Blocks) * int64(stat.Bsize)
	return free, total, nil
}
