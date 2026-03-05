package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Share represents a file sharing record in the database.
type Share struct {
	ID           int64
	Token        string
	FilePath     string
	OriginalName string
	SizeBytes    int64
	Uploader     string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Downloaded   int
	SHA256       string
}

// DB wraps the underlying sql.DB connection.
type DB struct {
	conn *sql.DB
}

// InitDB opens (or creates) the SQLite database at dbPath and runs migrations.
func InitDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrate(conn); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &DB{conn: conn}, nil
}

func migrate(conn *sql.DB) error {
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS shares (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			token         TEXT    NOT NULL UNIQUE,
			file_path     TEXT    NOT NULL,
			original_name TEXT    NOT NULL,
			size_bytes    INTEGER NOT NULL DEFAULT 0,
			uploader      TEXT    NOT NULL,
			created_at    DATETIME NOT NULL,
			expires_at    DATETIME NOT NULL,
			downloaded    INTEGER NOT NULL DEFAULT 0,
			sha256        TEXT    NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_shares_token    ON shares(token);
		CREATE INDEX IF NOT EXISTS idx_shares_uploader ON shares(uploader);
		CREATE INDEX IF NOT EXISTS idx_shares_expires  ON shares(expires_at);
	`)
	if err != nil {
		return err
	}
	// Add sha256 column to existing databases that pre-date this migration.
	// Ignore the error only when the column already exists.
	if _, err := conn.Exec(`ALTER TABLE shares ADD COLUMN sha256 TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add sha256 column: %w", err)
		}
	}
	return nil
}

// CreateShare inserts a new share record and returns the created Share.
func (db *DB) CreateShare(token, filePath, originalName string, sizeBytes int64, uploader string, expiresAt time.Time, sha256 string) (*Share, error) {
	now := time.Now().UTC()
	res, err := db.conn.Exec(
		`INSERT INTO shares (token, file_path, original_name, size_bytes, uploader, created_at, expires_at, sha256)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		token, filePath, originalName, sizeBytes, uploader, now, expiresAt.UTC(), sha256,
	)
	if err != nil {
		return nil, fmt.Errorf("create share: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Share{
		ID:           id,
		Token:        token,
		FilePath:     filePath,
		OriginalName: originalName,
		SizeBytes:    sizeBytes,
		Uploader:     uploader,
		CreatedAt:    now,
		ExpiresAt:    expiresAt.UTC(),
		SHA256:       sha256,
	}, nil
}

// GetShareByToken retrieves a share by its public token.
func (db *DB) GetShareByToken(token string) (*Share, error) {
	row := db.conn.QueryRow(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256
		 FROM shares WHERE token = ?`, token)
	return scanShare(row)
}

// ListSharesByUser returns all shares owned by the given uploader.
func (db *DB) ListSharesByUser(uploader string) ([]*Share, error) {
	rows, err := db.conn.Query(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256
		 FROM shares WHERE uploader = ? ORDER BY created_at DESC`, uploader)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []*Share
	for rows.Next() {
		s, err := scanShareRow(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, s)
	}
	return shares, rows.Err()
}

// GetExpiredShares returns all shares whose expiry has passed.
func (db *DB) GetExpiredShares() ([]*Share, error) {
	rows, err := db.conn.Query(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256
		 FROM shares WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []*Share
	for rows.Next() {
		s, err := scanShareRow(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, s)
	}
	return shares, rows.Err()
}

// DeleteShare removes a share record by its token.
func (db *DB) DeleteShare(token string) error {
	_, err := db.conn.Exec(`DELETE FROM shares WHERE token = ?`, token)
	return err
}

// IncrementDownload bumps the download counter.
func (db *DB) IncrementDownload(token string) {
	db.conn.Exec(`UPDATE shares SET downloaded = downloaded + 1 WHERE token = ?`, token)
}

// ── Scan helpers ────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanShare(row *sql.Row) (*Share, error) {
	s := &Share{}
	err := row.Scan(&s.ID, &s.Token, &s.FilePath, &s.OriginalName,
		&s.SizeBytes, &s.Uploader, &s.CreatedAt, &s.ExpiresAt, &s.Downloaded, &s.SHA256)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func scanShareRow(rows *sql.Rows) (*Share, error) {
	s := &Share{}
	err := rows.Scan(&s.ID, &s.Token, &s.FilePath, &s.OriginalName,
		&s.SizeBytes, &s.Uploader, &s.CreatedAt, &s.ExpiresAt, &s.Downloaded, &s.SHA256)
	if err != nil {
		return nil, err
	}
	return s, nil
}
