package database

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() error: %v", err)
	}
	return db
}

func TestInitDB_CreatesSchema(t *testing.T) {
	db := newTestDB(t)
	count, err := db.CountShares()
	if err != nil {
		t.Fatalf("CountShares() error: %v", err)
	}
	if count != 0 {
		t.Errorf("CountShares() on fresh DB = %d, want 0", count)
	}
}

func TestCreateShare_GetShareByToken_DeleteShare(t *testing.T) {
	db := newTestDB(t)
	expires := time.Now().Add(24 * time.Hour)

	created, err := db.CreateShare("tok-1", "/tmp/file.txt", "file.txt", 1234, "alice", expires, "deadbeef", "", 0)
	if err != nil {
		t.Fatalf("CreateShare() error: %v", err)
	}
	if created.ID == 0 {
		t.Error("CreateShare() returned zero ID")
	}

	got, err := db.GetShareByToken("tok-1")
	if err != nil {
		t.Fatalf("GetShareByToken() error: %v", err)
	}
	if got.Token != "tok-1" || got.OriginalName != "file.txt" || got.SizeBytes != 1234 || got.Uploader != "alice" {
		t.Errorf("GetShareByToken() = %+v, want matching fields to CreateShare()", got)
	}
	if got.SHA256 != "deadbeef" {
		t.Errorf("SHA256 = %q, want %q", got.SHA256, "deadbeef")
	}

	if err := db.DeleteShare("tok-1"); err != nil {
		t.Fatalf("DeleteShare() error: %v", err)
	}
	if _, err := db.GetShareByToken("tok-1"); err == nil {
		t.Error("GetShareByToken() after delete: want error, got nil")
	}
}

func TestListSharesByUser(t *testing.T) {
	db := newTestDB(t)
	expires := time.Now().Add(24 * time.Hour)

	if _, err := db.CreateShare("a1", "/p1", "f1.txt", 10, "alice", expires, "", "", 0); err != nil {
		t.Fatalf("CreateShare a1: %v", err)
	}
	if _, err := db.CreateShare("a2", "/p2", "f2.txt", 20, "alice", expires, "", "", 0); err != nil {
		t.Fatalf("CreateShare a2: %v", err)
	}
	if _, err := db.CreateShare("b1", "/p3", "f3.txt", 30, "bob", expires, "", "", 0); err != nil {
		t.Fatalf("CreateShare b1: %v", err)
	}

	shares, err := db.ListSharesByUser("alice")
	if err != nil {
		t.Fatalf("ListSharesByUser() error: %v", err)
	}
	if len(shares) != 2 {
		t.Fatalf("ListSharesByUser(alice) len = %d, want 2", len(shares))
	}
}

func TestGetUserTotalBytes(t *testing.T) {
	db := newTestDB(t)
	expires := time.Now().Add(24 * time.Hour)

	if _, err := db.CreateShare("t1", "/p1", "f1.txt", 100, "carol", expires, "", "", 0); err != nil {
		t.Fatalf("CreateShare t1: %v", err)
	}
	if _, err := db.CreateShare("t2", "/p2", "f2.txt", 250, "carol", expires, "", "", 0); err != nil {
		t.Fatalf("CreateShare t2: %v", err)
	}

	total, err := db.GetUserTotalBytes("carol")
	if err != nil {
		t.Fatalf("GetUserTotalBytes() error: %v", err)
	}
	if total != 350 {
		t.Errorf("GetUserTotalBytes(carol) = %d, want 350", total)
	}

	total, err = db.GetUserTotalBytes("nobody")
	if err != nil {
		t.Fatalf("GetUserTotalBytes(nobody) error: %v", err)
	}
	if total != 0 {
		t.Errorf("GetUserTotalBytes(nobody) = %d, want 0", total)
	}
}

func TestIncrementDownload(t *testing.T) {
	db := newTestDB(t)
	expires := time.Now().Add(24 * time.Hour)

	if _, err := db.CreateShare("dl1", "/p", "f.txt", 10, "dave", expires, "", "", 0); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	db.IncrementDownload("dl1")
	db.IncrementDownload("dl1")

	got, err := db.GetShareByToken("dl1")
	if err != nil {
		t.Fatalf("GetShareByToken() error: %v", err)
	}
	if got.Downloaded != 2 {
		t.Errorf("Downloaded = %d, want 2", got.Downloaded)
	}
}

func TestGetExpiredShares(t *testing.T) {
	db := newTestDB(t)
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	if _, err := db.CreateShare("exp1", "/p1", "f1.txt", 10, "eve", past, "", "", 0); err != nil {
		t.Fatalf("CreateShare exp1: %v", err)
	}
	if _, err := db.CreateShare("active1", "/p2", "f2.txt", 10, "eve", future, "", "", 0); err != nil {
		t.Fatalf("CreateShare active1: %v", err)
	}

	expired, err := db.GetExpiredShares()
	if err != nil {
		t.Fatalf("GetExpiredShares() error: %v", err)
	}
	if len(expired) != 1 || expired[0].Token != "exp1" {
		t.Fatalf("GetExpiredShares() = %v, want only [exp1]", expired)
	}
}

func TestUploadSession_CRUD(t *testing.T) {
	db := newTestDB(t)
	expires := time.Now().Add(1 * time.Hour)

	created, err := db.CreateUploadSession("sess-1", "frank", "big.zip", 1000, 10, 100, expires, 7, "", 0)
	if err != nil {
		t.Fatalf("CreateUploadSession() error: %v", err)
	}
	if created.SessionToken != "sess-1" {
		t.Errorf("SessionToken = %q, want %q", created.SessionToken, "sess-1")
	}

	got, err := db.GetUploadSession("sess-1")
	if err != nil {
		t.Fatalf("GetUploadSession() error: %v", err)
	}
	if got.Uploader != "frank" || got.TotalChunks != 10 {
		t.Errorf("GetUploadSession() = %+v, want matching CreateUploadSession() fields", got)
	}

	count, err := db.CountActiveUploadSessionsByUser("frank")
	if err != nil {
		t.Fatalf("CountActiveUploadSessionsByUser() error: %v", err)
	}
	if count != 1 {
		t.Errorf("CountActiveUploadSessionsByUser() = %d, want 1", count)
	}

	if err := db.DeleteUploadSession("sess-1"); err != nil {
		t.Fatalf("DeleteUploadSession() error: %v", err)
	}
	if _, err := db.GetUploadSession("sess-1"); err == nil {
		t.Error("GetUploadSession() after delete: want error, got nil")
	}
}

func TestMarkChunkReceived_Idempotent(t *testing.T) {
	db := newTestDB(t)
	expires := time.Now().Add(1 * time.Hour)

	if _, err := db.CreateUploadSession("sess-2", "gina", "f.zip", 500, 5, 100, expires, 7, "", 0); err != nil {
		t.Fatalf("CreateUploadSession() error: %v", err)
	}

	if err := db.MarkChunkReceived("sess-2", 0); err != nil {
		t.Fatalf("MarkChunkReceived(0) error: %v", err)
	}
	if err := db.MarkChunkReceived("sess-2", 2); err != nil {
		t.Fatalf("MarkChunkReceived(2) error: %v", err)
	}
	// Re-mark chunk 0: must stay idempotent, no duplicate entries.
	if err := db.MarkChunkReceived("sess-2", 0); err != nil {
		t.Fatalf("MarkChunkReceived(0) again error: %v", err)
	}

	got, err := db.GetUploadSession("sess-2")
	if err != nil {
		t.Fatalf("GetUploadSession() error: %v", err)
	}
	list := got.DoneChunkList()
	if len(list) != 2 || list[0] != 0 || list[1] != 2 {
		t.Errorf("DoneChunkList() = %v, want [0 2]", list)
	}
}

func TestRefreshUploadSession(t *testing.T) {
	db := newTestDB(t)
	original := time.Now().Add(1 * time.Minute)

	if _, err := db.CreateUploadSession("sess-3", "hank", "f.zip", 10, 1, 10, original, 1, "", 0); err != nil {
		t.Fatalf("CreateUploadSession() error: %v", err)
	}

	if err := db.RefreshUploadSession("sess-3", 3600); err != nil {
		t.Fatalf("RefreshUploadSession() error: %v", err)
	}

	got, err := db.GetUploadSession("sess-3")
	if err != nil {
		t.Fatalf("GetUploadSession() error: %v", err)
	}
	if !got.ExpiresAt.After(original.Add(30 * time.Minute)) {
		t.Errorf("ExpiresAt = %v, want extended well beyond original %v", got.ExpiresAt, original)
	}
}

func TestGetStaleUploadSessions(t *testing.T) {
	db := newTestDB(t)
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	if _, err := db.CreateUploadSession("stale-1", "ivy", "f.zip", 10, 1, 10, past, 1, "", 0); err != nil {
		t.Fatalf("CreateUploadSession stale-1: %v", err)
	}
	if _, err := db.CreateUploadSession("fresh-1", "ivy", "f.zip", 10, 1, 10, future, 1, "", 0); err != nil {
		t.Fatalf("CreateUploadSession fresh-1: %v", err)
	}

	stale, err := db.GetStaleUploadSessions()
	if err != nil {
		t.Fatalf("GetStaleUploadSessions() error: %v", err)
	}
	if len(stale) != 1 || stale[0].SessionToken != "stale-1" {
		t.Fatalf("GetStaleUploadSessions() = %v, want only [stale-1]", stale)
	}
}

func TestParseEncodeDoneChunks_Roundtrip(t *testing.T) {
	original := map[int]struct{}{0: {}, 5: {}, 3: {}}
	encoded := encodeDoneChunks(original)
	decoded := parseDoneChunks(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), len(original))
	}
	for k := range original {
		if _, ok := decoded[k]; !ok {
			t.Errorf("decoded missing key %d", k)
		}
	}
}

func TestParseDoneChunks_EmptyString(t *testing.T) {
	m := parseDoneChunks("")
	if len(m) != 0 {
		t.Errorf("parseDoneChunks(\"\") = %v, want empty map", m)
	}
}
