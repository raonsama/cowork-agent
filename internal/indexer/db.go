// Package indexer manages the SQLite-backed code symbol index.
package indexer

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite connection and provides FTS5-backed search.
type DB struct {
	conn *sql.DB
	path string
}

// OpenDB opens (or creates) the index database at the given path.
// MaxOpenConns(1) serialises all writes through a single connection,
// preventing "database is locked" errors under concurrent indexing.
func OpenDB(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	// _busy_timeout gives transient lock waits up to 5 s before erroring.
	dsn := path + "?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=4000&_busy_timeout=5000"
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Single connection prevents concurrent-write "database is locked" errors.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	db := &DB{conn: conn, path: path}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// migrate creates the schema if it doesn't exist.
func (db *DB) migrate() error {
	schema := `
	-- File metadata table
	CREATE TABLE IF NOT EXISTS files (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		path        TEXT    NOT NULL UNIQUE,
		ext         TEXT    NOT NULL,
		size        INTEGER NOT NULL,
		mod_time    INTEGER NOT NULL,
		hash        TEXT    NOT NULL,
		indexed_at  INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
	CREATE INDEX IF NOT EXISTS idx_files_hash ON files(hash);

	-- Function/symbol signatures
	CREATE TABLE IF NOT EXISTS symbols (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id     INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
		name        TEXT    NOT NULL,
		kind        TEXT    NOT NULL, -- func, type, const, var, class, etc.
		line_start  INTEGER NOT NULL,
		line_end    INTEGER NOT NULL,
		signature   TEXT    NOT NULL,
		body        TEXT    NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file_id);
	CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);

	-- FTS5 virtual table for full-text search
	CREATE VIRTUAL TABLE IF NOT EXISTS symbols_fts USING fts5(
		name,
		signature,
		body,
		content='symbols',
		content_rowid='id',
		tokenize='unicode61 remove_diacritics 1'
	);

	-- Triggers to keep FTS in sync
	CREATE TRIGGER IF NOT EXISTS symbols_ai AFTER INSERT ON symbols BEGIN
		INSERT INTO symbols_fts(rowid, name, signature, body)
		VALUES (new.id, new.name, new.signature, new.body);
	END;
	CREATE TRIGGER IF NOT EXISTS symbols_ad AFTER DELETE ON symbols BEGIN
		INSERT INTO symbols_fts(symbols_fts, rowid, name, signature, body)
		VALUES ('delete', old.id, old.name, old.signature, old.body);
	END;
	CREATE TRIGGER IF NOT EXISTS symbols_au AFTER UPDATE ON symbols BEGIN
		INSERT INTO symbols_fts(symbols_fts, rowid, name, signature, body)
		VALUES ('delete', old.id, old.name, old.signature, old.body);
		INSERT INTO symbols_fts(rowid, name, signature, body)
		VALUES (new.id, new.name, new.signature, new.body);
	END;
	`

	_, err := db.conn.Exec(schema)
	return err
}

// FileRecord holds metadata for an indexed file.
type FileRecord struct {
	ID        int64
	Path      string
	Ext       string
	Size      int64
	ModTime   int64
	Hash      string
	IndexedAt int64
}

// SymbolRecord holds a parsed symbol from source code.
type SymbolRecord struct {
	ID        int64
	FileID    int64
	Name      string
	Kind      string
	LineStart int
	LineEnd   int
	Signature string
	Body      string
}

// UpsertFile inserts or updates a file record, returning its row ID.
// Uses a post-upsert SELECT to handle the ON CONFLICT path correctly
// (LastInsertId returns 0 for SQLite UPDATE branches).
func (db *DB) UpsertFile(f *FileRecord) (int64, error) {
	_, err := db.conn.Exec(`
		INSERT INTO files (path, ext, size, mod_time, hash, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			ext=excluded.ext, size=excluded.size,
			mod_time=excluded.mod_time, hash=excluded.hash,
			indexed_at=excluded.indexed_at
	`, f.Path, f.Ext, f.Size, f.ModTime, f.Hash, f.IndexedAt)
	if err != nil {
		return 0, err
	}

	var id int64
	if err := db.conn.QueryRow(`SELECT id FROM files WHERE path = ?`, f.Path).Scan(&id); err != nil {
		return 0, fmt.Errorf("get file id for %q: %w", f.Path, err)
	}
	return id, nil
}

// FileNeedsReindex returns true if the file at path has changed since last index.
func (db *DB) FileNeedsReindex(path, hash string, modTime int64) (bool, int64) {
	var id int64
	var storedHash string
	var storedMod int64
	err := db.conn.QueryRow(
		`SELECT id, hash, mod_time FROM files WHERE path = ?`, path,
	).Scan(&id, &storedHash, &storedMod)
	if err != nil {
		return true, 0
	}
	return storedHash != hash || storedMod != modTime, id
}

// DeleteSymbolsForFile removes all symbols belonging to a file.
func (db *DB) DeleteSymbolsForFile(fileID int64) error {
	_, err := db.conn.Exec(`DELETE FROM symbols WHERE file_id = ?`, fileID)
	return err
}

// InsertSymbol adds a symbol record.
func (db *DB) InsertSymbol(s *SymbolRecord) error {
	_, err := db.conn.Exec(`
		INSERT INTO symbols (file_id, name, kind, line_start, line_end, signature, body)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, s.FileID, s.Name, s.Kind, s.LineStart, s.LineEnd, s.Signature, s.Body)
	return err
}

// SearchResult is a ranked FTS search result.
type SearchResult struct {
	FilePath  string
	Name      string
	Kind      string
	Signature string
	Body      string
	Score     float64
}

// Search performs FTS5 search and returns ranked results.
func (db *DB) Search(query string, limit int) ([]SearchResult, error) {
	rows, err := db.conn.Query(`
		SELECT f.path, s.name, s.kind, s.signature, s.body,
		       bm25(symbols_fts) AS score
		FROM symbols_fts
		JOIN symbols s ON s.id = symbols_fts.rowid
		JOIN files   f ON f.id = s.file_id
		WHERE symbols_fts MATCH ?
		ORDER BY score
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.FilePath, &r.Name, &r.Kind, &r.Signature, &r.Body, &r.Score); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// Stats returns basic index statistics.
func (db *DB) Stats() (files, symbols int64) {
	db.conn.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&files)
	db.conn.QueryRow(`SELECT COUNT(*) FROM symbols`).Scan(&symbols)
	return
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}
