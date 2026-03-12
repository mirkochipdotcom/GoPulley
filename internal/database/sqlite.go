package database

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
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
	PasswordHash string
	MaxDownloads int
}

// UploadSession represents an in-progress chunked upload.
type UploadSession struct {
	SessionToken string
	Uploader     string
	OriginalName string
	TotalSize    int64
	TotalChunks  int
	ChunkSize    int64
	// ChunksDone is a comma-separated list of received chunk indices (e.g. "0,1,3").
	ChunksDone   string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Days         int
	PasswordHash string
	MaxDownloads int
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
			sha256        TEXT    NOT NULL DEFAULT '',
			password_hash TEXT    NOT NULL DEFAULT '',
			max_downloads INTEGER NOT NULL DEFAULT 0
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
	if _, err := conn.Exec(`ALTER TABLE shares ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add password_hash column: %w", err)
		}
	}
	if _, err := conn.Exec(`ALTER TABLE shares ADD COLUMN max_downloads INTEGER NOT NULL DEFAULT 0`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add max_downloads column: %w", err)
		}
	}

	// uploads_in_progress: tracks chunked upload sessions.
	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS uploads_in_progress (
			session_token  TEXT     NOT NULL PRIMARY KEY,
			uploader       TEXT     NOT NULL,
			original_name  TEXT     NOT NULL,
			total_size     INTEGER  NOT NULL DEFAULT 0,
			total_chunks   INTEGER  NOT NULL DEFAULT 0,
			chunk_size     INTEGER  NOT NULL DEFAULT 0,
			chunks_done    TEXT     NOT NULL DEFAULT '',
			created_at     DATETIME NOT NULL,
			expires_at     DATETIME NOT NULL,
			days           INTEGER  NOT NULL DEFAULT 7,
			password_hash  TEXT     NOT NULL DEFAULT '',
			max_downloads  INTEGER  NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_uip_uploader  ON uploads_in_progress(uploader);
		CREATE INDEX IF NOT EXISTS idx_uip_expires   ON uploads_in_progress(expires_at);
	`)
	if err != nil {
		return fmt.Errorf("create uploads_in_progress table: %w", err)
	}

	return nil
}

// CreateShare inserts a new share record and returns the created Share.
func (db *DB) CreateShare(token, filePath, originalName string, sizeBytes int64, uploader string, expiresAt time.Time, sha256 string, passwordHash string, maxDownloads int) (*Share, error) {
	now := time.Now().UTC()
	res, err := db.conn.Exec(
		`INSERT INTO shares (token, file_path, original_name, size_bytes, uploader, created_at, expires_at, sha256, password_hash, max_downloads)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token, filePath, originalName, sizeBytes, uploader, now, expiresAt.UTC(), sha256, passwordHash, maxDownloads,
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
		PasswordHash: passwordHash,
		MaxDownloads: maxDownloads,
	}, nil
}

// GetShareByToken retrieves a share by its public token.
func (db *DB) GetShareByToken(token string) (*Share, error) {
	row := db.conn.QueryRow(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256, password_hash, max_downloads
		 FROM shares WHERE token = ?`, token)
	return scanShare(row)
}

// ListSharesByUser returns all shares owned by the given uploader.
func (db *DB) ListSharesByUser(uploader string) ([]*Share, error) {
	rows, err := db.conn.Query(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256, password_hash, max_downloads
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

// ListAllShares returns all shares in the database (for admin users).
func (db *DB) ListAllShares() ([]*Share, error) {
	rows, err := db.conn.Query(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256, password_hash, max_downloads
		 FROM shares ORDER BY created_at DESC`)
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

// CountShares returns the total number of shares in the database.
func (db *DB) CountShares() (int, error) {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM shares`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ListTopSharesBySize returns the top N largest shares in the database.
func (db *DB) ListTopSharesBySize(limit int) ([]*Share, error) {
	rows, err := db.conn.Query(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256, password_hash, max_downloads
		 FROM shares ORDER BY size_bytes DESC LIMIT ?`, limit)
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

// ListTopSharesByFurthestExpiration returns the top N shares with the longest expiration time.
func (db *DB) ListTopSharesByFurthestExpiration(limit int) ([]*Share, error) {
	rows, err := db.conn.Query(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256, password_hash, max_downloads
		 FROM shares ORDER BY expires_at DESC LIMIT ?`, limit)
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

// GetUserTotalBytes calculates the sum of all size_bytes for a specific user.
func (db *DB) GetUserTotalBytes(uploader string) (int64, error) {
	var total sql.NullInt64
	err := db.conn.QueryRow(`SELECT SUM(size_bytes) FROM shares WHERE uploader = ?`, uploader).Scan(&total)
	if err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}

// GetExpiredShares returns all shares whose expiry has passed.
func (db *DB) GetExpiredShares() ([]*Share, error) {
	rows, err := db.conn.Query(
		`SELECT id, token, file_path, original_name, size_bytes, uploader, created_at, expires_at, downloaded, sha256, password_hash, max_downloads
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

// SetShareSHA256 stores the computed SHA-256 digest for the given share token.
func (db *DB) SetShareSHA256(token, sha256 string) error {
	_, err := db.conn.Exec(`UPDATE shares SET sha256 = ? WHERE token = ?`, sha256, token)
	if err != nil {
		return fmt.Errorf("update share sha256: %w", err)
	}
	return nil
}

// ── Scan helpers ────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanShare(row *sql.Row) (*Share, error) {
	s := &Share{}
	err := row.Scan(&s.ID, &s.Token, &s.FilePath, &s.OriginalName,
		&s.SizeBytes, &s.Uploader, &s.CreatedAt, &s.ExpiresAt, &s.Downloaded, &s.SHA256, &s.PasswordHash, &s.MaxDownloads)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func scanShareRow(rows *sql.Rows) (*Share, error) {
	s := &Share{}
	err := rows.Scan(&s.ID, &s.Token, &s.FilePath, &s.OriginalName,
		&s.SizeBytes, &s.Uploader, &s.CreatedAt, &s.ExpiresAt, &s.Downloaded, &s.SHA256, &s.PasswordHash, &s.MaxDownloads)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ── Upload session CRUD ──────────────────────────────────────────────────────

// CreateUploadSession inserts a new in-progress upload session.
func (db *DB) CreateUploadSession(sessionToken, uploader, originalName string, totalSize int64, totalChunks int, chunkSize int64, expiresAt time.Time, days int, passwordHash string, maxDownloads int) (*UploadSession, error) {
	now := time.Now().UTC()
	_, err := db.conn.Exec(
		`INSERT INTO uploads_in_progress
		 (session_token, uploader, original_name, total_size, total_chunks, chunk_size, chunks_done, created_at, expires_at, days, password_hash, max_downloads)
		 VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?)`,
		sessionToken, uploader, originalName, totalSize, totalChunks, chunkSize, now, expiresAt.UTC(), days, passwordHash, maxDownloads,
	)
	if err != nil {
		return nil, fmt.Errorf("create upload session: %w", err)
	}
	return &UploadSession{
		SessionToken: sessionToken,
		Uploader:     uploader,
		OriginalName: originalName,
		TotalSize:    totalSize,
		TotalChunks:  totalChunks,
		ChunkSize:    chunkSize,
		ChunksDone:   "",
		CreatedAt:    now,
		ExpiresAt:    expiresAt.UTC(),
		Days:         days,
		PasswordHash: passwordHash,
		MaxDownloads: maxDownloads,
	}, nil
}

// GetUploadSession retrieves an in-progress upload session by its token.
func (db *DB) GetUploadSession(sessionToken string) (*UploadSession, error) {
	row := db.conn.QueryRow(
		`SELECT session_token, uploader, original_name, total_size, total_chunks, chunk_size, chunks_done, created_at, expires_at, days, password_hash, max_downloads
		 FROM uploads_in_progress WHERE session_token = ?`, sessionToken)
	s := &UploadSession{}
	err := row.Scan(&s.SessionToken, &s.Uploader, &s.OriginalName, &s.TotalSize, &s.TotalChunks, &s.ChunkSize,
		&s.ChunksDone, &s.CreatedAt, &s.ExpiresAt, &s.Days, &s.PasswordHash, &s.MaxDownloads)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// MarkChunkReceived appends the given chunk index to the chunks_done list (idempotent).
func (db *DB) MarkChunkReceived(sessionToken string, index int) error {
	s, err := db.GetUploadSession(sessionToken)
	if err != nil {
		return fmt.Errorf("get session for mark: %w", err)
	}
	// Parse existing indices
	existing := parseDoneChunks(s.ChunksDone)
	if _, ok := existing[index]; ok {
		return nil // already recorded — idempotent
	}
	existing[index] = struct{}{}
	newDone := encodeDoneChunks(existing)
	_, err = db.conn.Exec(`UPDATE uploads_in_progress SET chunks_done = ? WHERE session_token = ?`, newDone, sessionToken)
	return err
}

// CountActiveUploadSessionsByUser returns the number of active (non-expired) upload sessions for a user.
func (db *DB) CountActiveUploadSessionsByUser(uploader string) (int, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM uploads_in_progress WHERE uploader = ? AND expires_at > ?`,
		uploader, time.Now().UTC(),
	).Scan(&count)
	return count, err
}

// ListActiveUploadSessionsByUser returns active (non-expired) upload sessions for a user.
func (db *DB) ListActiveUploadSessionsByUser(uploader string) ([]*UploadSession, error) {
	rows, err := db.conn.Query(
		`SELECT session_token, uploader, original_name, total_size, total_chunks, chunk_size, chunks_done, created_at, expires_at, days, password_hash, max_downloads
		 FROM uploads_in_progress WHERE uploader = ? AND expires_at > ? ORDER BY created_at DESC`,
		uploader, time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*UploadSession
	for rows.Next() {
		s := &UploadSession{}
		if err := rows.Scan(&s.SessionToken, &s.Uploader, &s.OriginalName, &s.TotalSize, &s.TotalChunks, &s.ChunkSize,
			&s.ChunksDone, &s.CreatedAt, &s.ExpiresAt, &s.Days, &s.PasswordHash, &s.MaxDownloads); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// GetStaleUploadSessions returns upload sessions whose expires_at is in the past.
func (db *DB) GetStaleUploadSessions() ([]*UploadSession, error) {
	rows, err := db.conn.Query(
		`SELECT session_token, uploader, original_name, total_size, total_chunks, chunk_size, chunks_done, created_at, expires_at, days, password_hash, max_downloads
		 FROM uploads_in_progress WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*UploadSession
	for rows.Next() {
		s := &UploadSession{}
		if err := rows.Scan(&s.SessionToken, &s.Uploader, &s.OriginalName, &s.TotalSize, &s.TotalChunks, &s.ChunkSize,
			&s.ChunksDone, &s.CreatedAt, &s.ExpiresAt, &s.Days, &s.PasswordHash, &s.MaxDownloads); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// DeleteUploadSession removes an in-progress upload session record.
func (db *DB) DeleteUploadSession(sessionToken string) error {
	_, err := db.conn.Exec(`DELETE FROM uploads_in_progress WHERE session_token = ?`, sessionToken)
	return err
}

// RefreshUploadSession extends the expiration time of an active upload session by the given TTL duration.
func (db *DB) RefreshUploadSession(sessionToken string, ttlSeconds int) error {
	newExpiresAt := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second)
	_, err := db.conn.Exec(
		`UPDATE uploads_in_progress SET expires_at = ? WHERE session_token = ? AND expires_at > ?`,
		newExpiresAt, sessionToken, time.Now().UTC(),
	)
	return err
}

// ── Done-chunks encoding helpers ─────────────────────────────────────────────

func parseDoneChunks(s string) map[int]struct{} {
	m := make(map[int]struct{})
	if s == "" {
		return m
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i, err := strconv.Atoi(part); err == nil {
			m[i] = struct{}{}
		}
	}
	return m
}

func encodeDoneChunks(m map[int]struct{}) string {
	parts := make([]string, 0, len(m))
	for i := range m {
		parts = append(parts, strconv.Itoa(i))
	}
	return strings.Join(parts, ",")
}

// DoneChunkList converts the ChunksDone string into a sorted slice of indices.
func (s *UploadSession) DoneChunkList() []int {
	m := parseDoneChunks(s.ChunksDone)
	list := make([]int, 0, len(m))
	for i := range m {
		list = append(list, i)
	}
	sort.Ints(list)
	return list
}
