package localdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a sqlite DB used as a simple key/value store storing JSON blobs.
// Two tables: kv(collection TEXT, key TEXT, value BLOB) and logs(collection TEXT, key TEXT, value BLOB).
// This is intentionally simple and avoids BoltDB file-lock timeouts when only a single process runs.
type DB struct{ db *sql.DB }

// Open opens/creates the sqlite database file under the provided state directory.
func Open(stateDir string) (*DB, error) {
	if stateDir == "" {
		stateDir = "."
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, "guildnet.sqlite")
	dsn := path
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Set reasonable pragmas
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		// non-fatal
	}
	// Ensure schema
	schema := []string{
		`CREATE TABLE IF NOT EXISTS kv (collection TEXT NOT NULL, key TEXT NOT NULL, value BLOB, PRIMARY KEY(collection, key))`,
		`CREATE TABLE IF NOT EXISTS logs (collection TEXT NOT NULL, key TEXT NOT NULL, value BLOB, PRIMARY KEY(collection, key))`,
	}
	for _, s := range schema {
		if _, err := sqlDB.Exec(s); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("init sqlite schema: %w", err)
		}
	}
	return &DB{db: sqlDB}, nil
}

func (d *DB) Close() error { return d.db.Close() }

func (d *DB) EnsureBuckets(names ...string) error {
	// No-op for sqlite; tables are global. Return nil for compatibility.
	return nil
}

func (d *DB) Put(collection, k string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`INSERT INTO kv(collection,key,value) VALUES(?,?,?) ON CONFLICT(collection,key) DO UPDATE SET value=excluded.value`, collection, k, b)
	return err
}

func (d *DB) Get(collection, k string, out any) error {
	row := d.db.QueryRow(`SELECT value FROM kv WHERE collection=? AND key=?`, collection, k)
	var b []byte
	if err := row.Scan(&b); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("not found")
		}
		return err
	}
	return json.Unmarshal(b, out)
}

func (d *DB) Delete(collection, k string) error {
	_, err := d.db.Exec(`DELETE FROM kv WHERE collection=? AND key=?`, collection, k)
	return err
}

func (d *DB) List(collection string, out any) error {
	rows, err := d.db.Query(`SELECT value FROM kv WHERE collection=?`, collection)
	if err != nil {
		return err
	}
	defer rows.Close()
	arr := make([]json.RawMessage, 0)
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return err
		}
		arr = append(arr, append([]byte(nil), b...))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	bb, err := json.Marshal(arr)
	if err != nil {
		return err
	}
	return json.Unmarshal(bb, out)
}

func (d *DB) AppendLog(collection, k string, line []byte) error {
	// Fetch existing
	var cur []byte
	row := d.db.QueryRow(`SELECT value FROM logs WHERE collection=? AND key=?`, collection, k)
	switch err := row.Scan(&cur); err {
	case nil:
		// ok
	case sql.ErrNoRows:
		cur = nil
	default:
		return err
	}
	combined := append([]byte(nil), cur...)
	combined = append(combined, line...)
	if len(combined) == 0 || combined[len(combined)-1] != '\n' {
		combined = append(combined, '\n')
	}
	_, err := d.db.Exec(`INSERT INTO logs(collection,key,value) VALUES(?,?,?) ON CONFLICT(collection,key) DO UPDATE SET value=excluded.value`, collection, k, combined)
	return err
}

func (d *DB) ReadLog(collection, k string) ([]byte, error) {
	row := d.db.QueryRow(`SELECT value FROM logs WHERE collection=? AND key=?`, collection, k)
	var b []byte
	if err := row.Scan(&b); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return append([]byte(nil), b...), nil
}
