package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/theapemachine/errnie"
	_ "github.com/mattn/go-sqlite3"
)

const eventSchema = `
CREATE TABLE IF NOT EXISTS events (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind     TEXT    NOT NULL,
    endpoint TEXT    NOT NULL DEFAULT '',
    at       TEXT    NOT NULL,
    data     BLOB    NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_events_kind ON events(kind);
CREATE INDEX IF NOT EXISTS idx_events_at   ON events(at);
`

const endpointIndex = `
CREATE INDEX IF NOT EXISTS idx_events_endpoint ON events(endpoint);
`

const endpointMigration = `
ALTER TABLE events ADD COLUMN endpoint TEXT NOT NULL DEFAULT '';
`

/*
SQLite is the default Repository engine. It persists every WriteEvent as one
row in a single kind-tagged event table, so replay is a single ordered scan per
kind rather than a per-domain table sprawl. The endpoint column names the origin
stream (the websocket URL for raw frames, the layer name for stage snapshots)
without being welded onto the payload, so the data column stays verbatim and
each frame is addressable by its source. The connection is opened once and
serialized (one writer) with WAL and a busy timeout so the writer never blocks
readers and never loses a record to a transient lock.
*/
type SQLite struct {
	database *sql.DB
}

/*
NewSQLite opens (creating if absent) the SQLite database backing the store. An
empty path is a validation error; the engine owns schema creation on first use.
*/
func NewSQLite(path string) (*SQLite, error) {
	if path == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite path required",
			nil,
		))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: mkdir failed for %s [%s]", filepath.Dir(path), err.Error()),
			err,
		))
	}

	database, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: open failed for %s [%s]", path, err.Error()),
			err,
		))
	}

	database.SetMaxOpenConns(1)

	store := &SQLite{database: database}

	if err := store.EnsureSchema(); err != nil {
		_ = database.Close()
		return nil, err
	}

	return store, nil
}

/*
EnsureSchema creates the event table and its indexes if they do not already
exist, and migrates pre-endpoint databases to carry the endpoint column. It is
safe to call repeatedly.
*/
func (store *SQLite) EnsureSchema() error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if _, err := store.database.Exec(eventSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure schema failed",
			err,
		))
	}

	if err := store.migrateEndpoint(); err != nil {
		return err
	}

	// The endpoint index depends on the column that the migration above may
	// just have added, so it is created only after the column is guaranteed.
	if _, err := store.database.Exec(endpointIndex); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure endpoint index failed",
			err,
		))
	}

	return nil
}

/*
migrateEndpoint adds the endpoint column to databases created before the
endpoint/payload split. The CREATE TABLE IF NOT EXISTS above leaves an existing
table untouched, so a pre-split database is upgraded here instead.
*/
func (store *SQLite) migrateEndpoint() error {
	columns, err := store.database.Query("PRAGMA table_info(events)")

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: inspect events schema for endpoint migration",
			err,
		))
	}

	defer columns.Close()

	for columns.Next() {
		var (
			colID     int
			colName   string
			colType   string
			colNotNull int
			colDefault sql.NullString
			colPK      int
		)

		if err := columns.Scan(
			&colID, &colName, &colType, &colNotNull, &colDefault, &colPK,
		); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"store: read events column metadata",
				err,
			))
		}

		if colName == "endpoint" {
			return nil
		}
	}

	if _, err := store.database.Exec(endpointMigration); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: add endpoint column to events",
			err,
		))
	}

	return nil
}

/*
WriteFrame persists one raw transport frame. The payload is stored verbatim —
nothing here inspects or reshapes it — tagged with the endpoint it arrived from
and the kind (channel/feed) that produced it.
*/
func (store *SQLite) WriteFrame(endpoint, kind string, payload []byte, at time.Time) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if _, err := store.database.Exec(
		"INSERT INTO events (kind, endpoint, at, data) VALUES (?, ?, ?, ?)",
		kind,
		endpoint,
		at.UTC().Format(time.RFC3339Nano),
		payload,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: write %s event failed [%s]", kind, err.Error()),
			err,
		))
	}

	return nil
}

/*
Close releases the database handle.
*/
func (store *SQLite) Close() error {
	if store == nil || store.database == nil {
		return nil
	}

	return store.database.Close()
}
