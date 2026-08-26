package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHumanSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1024 * 1024, "1.00 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
		{int64(2.5 * 1024 * 1024 * 1024), "2.50 GB"},
	}
	for _, tc := range cases {
		if got := HumanSize(tc.bytes); got != tc.want {
			t.Errorf("HumanSize(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestChunkDir_SanitizesTraversal(t *testing.T) {
	uploadDir := t.TempDir()

	got := ChunkDir(uploadDir, "../../etc/passwd")
	want := filepath.Join(uploadDir, ".chunks", "passwd")
	if got != want {
		t.Errorf("ChunkDir() = %q, want %q", got, want)
	}
}

func TestChunkFilePath(t *testing.T) {
	uploadDir := t.TempDir()
	got := ChunkFilePath(uploadDir, "session1", 3)
	want := filepath.Join(uploadDir, ".chunks", "session1", "chunk_000003")
	if got != want {
		t.Errorf("ChunkFilePath() = %q, want %q", got, want)
	}
}

func TestSaveChunk_ComposeChunks_Roundtrip(t *testing.T) {
	uploadDir := t.TempDir()
	session := "sess-abc"

	parts := []string{"hello ", "world", "!"}
	for i, p := range parts {
		n, err := SaveChunk(uploadDir, session, i, strings.NewReader(p))
		if err != nil {
			t.Fatalf("SaveChunk(%d) error: %v", i, err)
		}
		if n != int64(len(p)) {
			t.Errorf("SaveChunk(%d) wrote %d bytes, want %d", i, n, len(p))
		}
	}

	dest := filepath.Join(uploadDir, "final.txt")
	if err := ComposeChunks(uploadDir, session, len(parts), dest); err != nil {
		t.Fatalf("ComposeChunks() error: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read composed file: %v", err)
	}
	want := "hello world!"
	if string(got) != want {
		t.Errorf("composed content = %q, want %q", got, want)
	}

	if err := CleanupChunkDir(uploadDir, session); err != nil {
		t.Fatalf("CleanupChunkDir() error: %v", err)
	}
	if _, err := os.Stat(ChunkDir(uploadDir, session)); !os.IsNotExist(err) {
		t.Error("chunk dir still exists after CleanupChunkDir()")
	}
}

func TestSaveChunk_IsIdempotent(t *testing.T) {
	uploadDir := t.TempDir()
	session := "sess-idem"

	if _, err := SaveChunk(uploadDir, session, 0, strings.NewReader("first")); err != nil {
		t.Fatalf("first SaveChunk error: %v", err)
	}
	if _, err := SaveChunk(uploadDir, session, 0, strings.NewReader("second")); err != nil {
		t.Fatalf("second SaveChunk error: %v", err)
	}

	got, err := os.ReadFile(ChunkFilePath(uploadDir, session, 0))
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("chunk content = %q, want overwritten value %q", got, "second")
	}
}

func TestComposeChunks_RejectsPathTraversal(t *testing.T) {
	uploadDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil.txt")

	err := ComposeChunks(uploadDir, "sess", 1, outside)
	if err == nil {
		t.Fatal("ComposeChunks() with dest outside uploadDir: want error, got nil")
	}
}

func TestComputeSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	content := []byte("the quick brown fox")
	if err := os.WriteFile(path, content, 0640); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	want := sha256.Sum256(content)
	wantHex := hex.EncodeToString(want[:])

	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("ComputeSHA256() error: %v", err)
	}
	if got != wantHex {
		t.Errorf("ComputeSHA256() = %q, want %q", got, wantHex)
	}
}

func TestComputeSHA256_MissingFile(t *testing.T) {
	_, err := ComputeSHA256(filepath.Join(t.TempDir(), "nope.bin"))
	if err == nil {
		t.Fatal("ComputeSHA256() on missing file: want error, got nil")
	}
}

func TestDeleteFile_RemovesParentDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "uuid-folder")
	if err := os.MkdirAll(subdir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	filePath := filepath.Join(subdir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0640); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := DeleteFile(filePath); err != nil {
		t.Fatalf("DeleteFile() error: %v", err)
	}
	if _, err := os.Stat(subdir); !os.IsNotExist(err) {
		t.Error("parent dir still exists after DeleteFile()")
	}
}
