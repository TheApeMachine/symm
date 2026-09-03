package broker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

const positionStoplossSchema = `
CREATE TABLE IF NOT EXISTS position_stoplosses (
    symbol TEXT NOT NULL,
    entry_at TEXT NOT NULL,
    state BLOB NOT NULL,
    PRIMARY KEY (symbol, entry_at)
) STRICT;`

/*
PositionStore persists the stoploss attached to each open position.
*/
type PositionStore struct {
	database *sql.DB
	queue    chan positionStoreOperation
	done     chan struct{}
	failed   chan struct{}

	stateMu   sync.RWMutex
	closed    bool
	closeOnce sync.Once
	errorMu   sync.RWMutex
	err       error
	batchSize int
	shedCount uint64
}

/*
ShedCount reports how many position write operations were dropped because the
queue was saturated.
*/
func (store *PositionStore) ShedCount() uint64 {
	if store == nil {
		return 0
	}

	return atomic.LoadUint64(&store.shedCount)
}

/*
NewPositionStore opens the SQLite database used by position recovery.
*/
func NewPositionStore(path string, queueDepth, batchSize int) (*PositionStore, error) {
	if path == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: path required",
			nil,
		))
	}

	if queueDepth < 1 || batchSize < 1 || batchSize > queueDepth {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: queue depth and batch size must be positive, with batch within queue",
			nil,
		))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: mkdir failed for %s [%s]", path, err.Error()),
			err,
		))
	}

	database, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: open failed for %s [%s]", path, err.Error()),
			err,
		))
	}

	database.SetMaxOpenConns(1)

	store := &PositionStore{
		database:  database,
		queue:     make(chan positionStoreOperation, queueDepth),
		done:      make(chan struct{}),
		failed:    make(chan struct{}),
		batchSize: batchSize,
	}

	if err := store.EnsureSchema(); err != nil {
		_ = database.Close()
		return nil, err
	}

	go store.runWriter()

	return store, nil
}

/*
EnsureSchema creates the required tables if they do not already exist, and
migrates position_stoplosses forward if it was created under the old
symbol-only schema (before entry_at existed). CREATE TABLE IF NOT EXISTS
does not add columns to an existing table, so a database from before this
column was introduced silently keeps its old shape forever without this step.
*/
func (store *PositionStore) EnsureSchema() error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	if err := store.migrateStoplossSchema(); err != nil {
		return err
	}

	if _, err := store.database.Exec(positionStoplossSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: schema failed",
			err,
		))
	}

	if _, err := store.database.Exec(positionTradesSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: trades schema failed",
			err,
		))
	}

	return nil
}

/*
migrateStoplossSchema renames an existing symbol-only position_stoplosses
table out of the way so the caller can recreate it under the current
(symbol, entry_at) schema. Only rows whose marshaled state already carries an
entry_at (saved after that field was introduced) are carried forward; a row
predating it can never be safely attributed to a specific lot, and recovery's
existing fallback synthesizes fresh protection for a position whose row is
missing, so dropping it here is not a regression in coverage — it is
identical to the missing-row path recovery already handles correctly. A
database that never had this table, or already has entry_at, is left alone.
*/
func (store *PositionStore) migrateStoplossSchema() error {
	var tableCount int

	if err := store.database.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='position_stoplosses'",
	).Scan(&tableCount); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: check for existing stoploss table failed",
			err,
		))
	}

	if tableCount == 0 {
		return nil
	}

	rows, err := store.database.Query("PRAGMA table_info(position_stoplosses)")

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: inspect stoploss schema failed",
			err,
		))
	}

	hasEntryAt := false

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			dflt       any
			pk         int
		)

		if scanErr := rows.Scan(&cid, &name, &columnType, &notNull, &dflt, &pk); scanErr != nil {
			_ = rows.Close()

			return errnie.Error(errnie.Err(
				errnie.IO,
				"position store: read stoploss column info failed",
				scanErr,
			))
		}

		if name == "entry_at" {
			hasEntryAt = true
		}
	}

	if err := rows.Err(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: row scan failed",
			err,
		))
	}

	if closeErr := rows.Close(); closeErr != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: close stoploss column info failed",
			closeErr,
		))
	}

	if hasEntryAt {
		return nil
	}

	tx, err := store.database.Begin()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: begin stoploss migration failed",
			err,
		))
	}

	if _, err := tx.Exec("ALTER TABLE position_stoplosses RENAME TO position_stoplosses_pre_entry_at"); err != nil {
		_ = tx.Rollback()

		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: rename legacy stoploss table failed",
			err,
		))
	}

	if _, err := tx.Exec(positionStoplossSchema); err != nil {
		_ = tx.Rollback()

		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: create migrated stoploss table failed",
			err,
		))
	}

	legacyRows, err := tx.Query("SELECT symbol, state FROM position_stoplosses_pre_entry_at")

	if err != nil {
		_ = tx.Rollback()

		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: read legacy stoploss rows failed",
			err,
		))
	}

	migrated := 0
	dropped := 0

	for legacyRows.Next() {
		var (
			symbol string
			state  []byte
		)

		if scanErr := legacyRows.Scan(&symbol, &state); scanErr != nil {
			_ = legacyRows.Close()
			_ = tx.Rollback()

			return errnie.Error(errnie.Err(
				errnie.IO,
				"position store: scan legacy stoploss row failed",
				scanErr,
			))
		}

		var decoded struct {
			EntryAt *time.Time `json:"entry_at"`
		}

		if jsonErr := json.Unmarshal(state, &decoded); jsonErr != nil || decoded.EntryAt == nil || decoded.EntryAt.IsZero() {
			dropped++

			continue
		}

		if _, insertErr := tx.Exec(
			"INSERT INTO position_stoplosses (symbol, entry_at, state) VALUES (?, ?, ?)",
			symbol,
			decoded.EntryAt.UTC().Format(time.RFC3339Nano),
			state,
		); insertErr != nil {
			_ = legacyRows.Close()
			_ = tx.Rollback()

			return errnie.Error(errnie.Err(
				errnie.IO,
				"position store: insert migrated stoploss row failed",
				insertErr,
			))
		}

		migrated++
	}

	if err := legacyRows.Err(); err != nil {
		_ = legacyRows.Close()
		_ = tx.Rollback()

		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: read legacy stoploss rows failed",
			err,
		))
	}

	if closeErr := legacyRows.Close(); closeErr != nil {
		_ = tx.Rollback()

		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: close legacy stoploss rows failed",
			closeErr,
		))
	}

	if _, err := tx.Exec("DROP TABLE position_stoplosses_pre_entry_at"); err != nil {
		_ = tx.Rollback()

		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: drop legacy stoploss table failed",
			err,
		))
	}

	if err := tx.Commit(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: commit stoploss migration failed",
			err,
		))
	}

	errnie.Info(fmt.Sprintf(
		"position store: migrated stoploss table to (symbol, entry_at); %d rows carried forward, %d rows without entry_at dropped",
		migrated, dropped,
	))

	return nil
}

/*
Save stores the current stoploss state, keyed by symbol and entry time. A
symbol is re-entered many times over a session; keying on symbol alone would
let a later entry's row be confused with — or silently overwrite — an
already-closed trade's, which is exactly what let a stale, already-triggered
stoploss survive to be read back as if it protected a different, still-open
position. EntryAt is required: every stoploss reaching persistence has gone
through RebindFill, which always stamps it.
*/
func (store *PositionStore) Save(stoploss *types.Stoploss) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	if stoploss.EntryAt == nil || stoploss.EntryAt.IsZero() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: stoploss entry time required to save "+stoploss.Symbol,
			nil,
		))
	}

	state, err := stoploss.MarshalState()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: marshal stoploss failed [%s]", err.Error()),
			err,
		))
	}

	entryAt := stoploss.EntryAt.UTC().Format(time.RFC3339Nano)

	return store.enqueue(positionStoreOperation{
		key: stoploss.Symbol,
		query: `
INSERT INTO position_stoplosses (symbol, entry_at, state) VALUES (?, ?, ?)
ON CONFLICT(symbol, entry_at) DO UPDATE SET state = excluded.state`,
		args:        []any{stoploss.Symbol, entryAt, state},
		description: "save stoploss for " + stoploss.Symbol,
	})
}

/*
Load returns the stoploss stored for a symbol at the given entry time, or nil
when none exists. A row from a different entry time on the same symbol — an
already-closed trade's leftover state — is never returned: recovery must not
mistake one lot's protection for another's.
*/
func (store *PositionStore) Load(
	ctx context.Context,
	symbol string,
	entryAt time.Time,
) (*types.Stoploss, error) {
	if store == nil || store.database == nil || symbol == "" || entryAt.IsZero() {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database, symbol, and entry time required",
			nil,
		))
	}

	if err := store.Sync(); err != nil {
		return nil, err
	}

	var state []byte
	err := store.database.QueryRow(
		"SELECT state FROM position_stoplosses WHERE symbol = ? AND entry_at = ?",
		symbol,
		entryAt.UTC().Format(time.RFC3339Nano),
	).Scan(&state)

	if isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"position store: load stoploss failed for %s [%s]",
					symbol, err.Error(),
				),
				schemaErr,
			))
		}

		return nil, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: load stoploss failed for %s [%s]", symbol, err.Error()),
			err,
		))
	}

	return types.RestoreStoploss(ctx, state)
}

/*
Delete removes every stored stoploss for a symbol after its position closes.
Deleting the whole symbol rather than just the closing entry's row also clears
any already-stale rows a prior session left behind for the same symbol.
*/
func (store *PositionStore) Delete(symbol string) error {
	if store == nil || store.database == nil || symbol == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	return store.enqueue(positionStoreOperation{
		key:         symbol,
		query:       "DELETE FROM position_stoplosses WHERE symbol = ?",
		args:        []any{symbol},
		description: "delete stoploss for " + symbol,
	})
}

/*
Close releases the SQLite database.
*/
func (store *PositionStore) Close() error {
	if store == nil {
		return nil
	}

	store.closeOnce.Do(func() {
		store.stateMu.Lock()
		store.closed = true
		close(store.queue)
		store.stateMu.Unlock()

		<-store.done

		if err := store.database.Close(); err != nil && store.Error() == nil {
			store.setError(err)
		}
	})

	return store.Error()
}

func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "no such table")
}
