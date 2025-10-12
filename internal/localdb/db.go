package localdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// DB is a simple embedded key-value database using BoltDB.
// Data is stored as JSON values in top-level buckets per collection.
// Keys are string IDs. All operations are ACID via Bolt's single-writer model.
type DB struct {
	b *bolt.DB
}

// Open opens/creates the database file under the provided state directory.
func Open(stateDir string) (*DB, error) {
	if stateDir == "" {
		stateDir = "."
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, "guildnet.bolt")
	b, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	return &DB{b: b}, nil
}

func (d *DB) Close() error { return d.b.Close() }

// EnsureBuckets makes sure the given collection buckets exist.
func (d *DB) EnsureBuckets(names ...string) error {
	return d.b.Update(func(tx *bolt.Tx) error {
		for _, n := range names {
			if _, err := tx.CreateBucketIfNotExists([]byte(n)); err != nil {
				return fmt.Errorf("create bucket %s: %w", n, err)
			}
		}
		return nil
	})
}

// Put stores v encoded as JSON under key k in collection bucket.
func (d *DB) Put(collection, k string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return d.b.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte(collection))
		if bk == nil {
			return fmt.Errorf("bucket not found: %s", collection)
		}
		return bk.Put([]byte(k), b)
	})
}

// Get decodes a JSON value with key k from collection into out.
func (d *DB) Get(collection, k string, out any) error {
	return d.b.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte(collection))
		if bk == nil {
			return fmt.Errorf("bucket not found: %s", collection)
		}
		v := bk.Get([]byte(k))
		if v == nil {
			return errors.New("not found")
		}
		return json.Unmarshal(v, out)
	})
}

// Delete removes a key from collection.
func (d *DB) Delete(collection, k string) error {
	return d.b.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte(collection))
		if bk == nil {
			return fmt.Errorf("bucket not found: %s", collection)
		}
		return bk.Delete([]byte(k))
	})
}

// List returns all values in a collection decoded as JSON into out slice pointer.
// out must be a pointer to a slice of a concrete type.
func (d *DB) List(collection string, out any) error {
	return d.b.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte(collection))
		if bk == nil {
			return fmt.Errorf("bucket not found: %s", collection)
		}
		arr := make([]json.RawMessage, 0)
		if err := bk.ForEach(func(k, v []byte) error {
			arr = append(arr, append([]byte(nil), v...))
			return nil
		}); err != nil {
			return err
		}
		b, err := json.Marshal(arr)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, out)
	})
}

// AppendLog appends a line (with trailing newline if missing) to value at key in collection.
// The value is stored as []byte, not JSON-encoded, to avoid large JSON arrays.
func (d *DB) AppendLog(collection, k string, line []byte) error {
	return d.b.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte(collection))
		if bk == nil {
			return fmt.Errorf("bucket not found: %s", collection)
		}
		cur := bk.Get([]byte(k))
		var combined []byte
		if len(cur) > 0 {
			combined = append(combined, cur...)
		}
		combined = append(combined, line...)
		if len(combined) == 0 || combined[len(combined)-1] != '\n' {
			combined = append(combined, '\n')
		}
		return bk.Put([]byte(k), combined)
	})
}

// ReadLog returns the raw bytes of a log blob.
func (d *DB) ReadLog(collection, k string) ([]byte, error) {
	var out []byte
	err := d.b.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte(collection))
		if bk == nil {
			return fmt.Errorf("bucket not found: %s", collection)
		}
		v := bk.Get([]byte(k))
		if v == nil {
			out = nil
			return nil
		}
		out = append([]byte(nil), v...)
		return nil
	})
	return out, err
}
