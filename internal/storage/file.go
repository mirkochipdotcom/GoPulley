package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

// ComputeSHA256 computes the SHA-256 hex digest of the file at the given path.
func ComputeSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file for sha256: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("compute sha256: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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

// ── Chunked upload helpers ───────────────────────────────────────────────────

// ChunkDir returns the directory where chunks for a session are stored.
func ChunkDir(uploadDir, session string) string {
	// Sanitize session to prevent path traversal (session should be a UUID).
	clean := filepath.Base(session)
	return filepath.Join(uploadDir, ".chunks", clean)
}

// ChunkFilePath returns the path for a specific chunk file within a session.
func ChunkFilePath(uploadDir, session string, index int) string {
	return filepath.Join(ChunkDir(uploadDir, session), fmt.Sprintf("chunk_%06d", index))
}

// SaveChunk writes raw bytes to the chunk file for the given session and index.
// It is idempotent: re-writing an existing chunk overwrites it.
func SaveChunk(uploadDir, session string, index int, data io.Reader) (int64, error) {
	dir := ChunkDir(uploadDir, session)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return 0, fmt.Errorf("mkdir chunk dir: %w", err)
	}
	// Guard against path traversal via session (double-check)
	chunkPath := ChunkFilePath(uploadDir, session, index)
	if !strings.HasPrefix(filepath.Clean(chunkPath), filepath.Clean(uploadDir)) {
		return 0, fmt.Errorf("invalid chunk path: path traversal detected")
	}

	f, err := os.OpenFile(chunkPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return 0, fmt.Errorf("create chunk file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, data)
	if err != nil {
		os.Remove(chunkPath)
		return 0, fmt.Errorf("write chunk: %w", err)
	}
	return n, nil
}

// ComposeChunks assembles totalChunks sequential chunk files from chunkDir into
// the file at destPath.  destPath is created (or truncated) by this call.
// The caller is responsible for cleaning up chunk files after a successful compose.
func ComposeChunks(uploadDir, session string, totalChunks int, destPath string) error {
	// Guard against path traversal in destPath
	if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(uploadDir)) {
		return fmt.Errorf("invalid dest path: path traversal detected")
	}

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("create dest file: %w", err)
	}
	defer dst.Close()

	for i := 0; i < totalChunks; i++ {
		chunkPath := ChunkFilePath(uploadDir, session, i)
		cf, err := os.Open(chunkPath)
		if err != nil {
			return fmt.Errorf("open chunk %d: %w", i, err)
		}
		_, copyErr := io.Copy(dst, cf)
		cf.Close()
		if copyErr != nil {
			return fmt.Errorf("copy chunk %d: %w", i, copyErr)
		}
	}
	return nil
}

// CleanupChunkDir removes the entire chunk directory for a session.
func CleanupChunkDir(uploadDir, session string) error {
	return os.RemoveAll(ChunkDir(uploadDir, session))
}
