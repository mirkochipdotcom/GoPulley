package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// SaveFile stores an uploaded file under uploadDir/<uuid>/<originalFilename>
// and returns the full path, original filename, and file size in bytes.
func SaveFile(header *multipart.FileHeader, uploadDir string) (filePath, originalName string, sizeBytes int64, err error) {
	src, err := header.Open()
	if err != nil {
		return "", "", 0, fmt.Errorf("open upload: %w", err)
	}
	defer src.Close()

	originalName = filepath.Base(header.Filename)
	dir := filepath.Join(uploadDir, uuid.New().String())
	if err = os.MkdirAll(dir, 0750); err != nil {
		return "", "", 0, fmt.Errorf("mkdir: %w", err)
	}

	filePath = filepath.Join(dir, originalName)
	dst, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return "", "", 0, fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()

	sizeBytes, err = io.Copy(dst, src)
	if err != nil {
		// Clean up partial file
		os.Remove(filePath)
		return "", "", 0, fmt.Errorf("write file: %w", err)
	}

	return filePath, originalName, sizeBytes, nil
}

// DeleteFile removes the file and its containing directory.
func DeleteFile(filePath string) error {
	// Remove the parent directory (which is the uuid folder)
	return os.RemoveAll(filepath.Dir(filePath))
}

// ServeFile streams the file to the HTTP response with appropriate headers
// for browser download (Content-Disposition: attachment).
func ServeFile(w http.ResponseWriter, r *http.Request, filePath, originalName string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, originalName))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Cache-Control", "no-store")

	_, err = io.Copy(w, f)
	return err
}

// HumanSize formats bytes into a human-readable string (KB / MB / GB).
func HumanSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
