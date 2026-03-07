//go:build windows

package storage

// GetDiskUsage returns free and total disk space in bytes for the specified path.
func GetDiskUsage(path string) (free, total int64, err error) {
	// Simple stub for local windows dev since the app runs in linux containers
	return 0, 0, nil
}
