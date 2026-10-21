package database

// Test di compatibilità per gli aggiornamenti di mattn/go-sqlite3 (in
// auto-merge dependabot): un DB di produzione esistente deve continuare ad
// aprirsi, migrare e restituire le date correttamente.

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestInitDB_ReopenIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("first InitDB() error: %v", err)
	}
	if _, err := db.CreateShare("tok-reopen", "/data/f", "f.txt", 10, "mario", time.Now().Add(time.Hour), "", "", 0); err != nil {
		t.Fatalf("CreateShare() error: %v", err)
	}
	db.conn.Close()

	// La migrazione ignora "duplicate column name": se il testo dell'errore
	// cambiasse con una nuova versione di SQLite, la riapertura fallirebbe.
	db, err = InitDB(dbPath)
	if err != nil {
		t.Fatalf("second InitDB() on existing DB error: %v", err)
	}
	if _, err := db.GetShareByToken("tok-reopen"); err != nil {
		t.Fatalf("GetShareByToken() after reopen error: %v", err)
	}
}

func TestInitDB_MigratesLegacySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	// Schema shares precedente alle colonne sha256/password_hash/max_downloads,
	// con una riga scritta nel formato DATETIME usato da go-sqlite3.
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE shares (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			token         TEXT    NOT NULL UNIQUE,
			file_path     TEXT    NOT NULL,
			original_name TEXT    NOT NULL,
			size_bytes    INTEGER NOT NULL DEFAULT 0,
			uploader      TEXT    NOT NULL,
			created_at    DATETIME NOT NULL,
			expires_at    DATETIME NOT NULL,
			downloaded    INTEGER NOT NULL DEFAULT 0
		);
		INSERT INTO shares (token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded)
		VALUES ('tok-legacy', '/data/uploads/a', 'a.pdf', 1234, 'mario',
		        '2025-01-02 03:04:05.123456789+00:00', '2025-01-09 03:04:05+00:00', 2);
	`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	conn.Close()

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() on legacy DB error: %v", err)
	}
	s, err := db.GetShareByToken("tok-legacy")
	if err != nil {
		t.Fatalf("GetShareByToken() error: %v", err)
	}

	wantCreated := time.Date(2025, 1, 2, 3, 4, 5, 123456789, time.UTC)
	wantExpires := time.Date(2025, 1, 9, 3, 4, 5, 0, time.UTC)
	if !s.CreatedAt.Equal(wantCreated) {
		t.Errorf("CreatedAt = %v, want %v", s.CreatedAt, wantCreated)
	}
	if !s.ExpiresAt.Equal(wantExpires) {
		t.Errorf("ExpiresAt = %v, want %v", s.ExpiresAt, wantExpires)
	}
	if s.Downloaded != 2 || s.SizeBytes != 1234 {
		t.Errorf("Downloaded/SizeBytes = %d/%d, want 2/1234", s.Downloaded, s.SizeBytes)
	}
	if s.SHA256 != "" || s.PasswordHash != "" || s.MaxDownloads != 0 {
		t.Errorf("migrated columns must default to empty/0, got %q %q %d", s.SHA256, s.PasswordHash, s.MaxDownloads)
	}
}
