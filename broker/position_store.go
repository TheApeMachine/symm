package broker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

const thesisCheckpointSchema = `
CREATE TABLE IF NOT EXISTS thesis_checkpoints (
    id INTEGER PRIMARY KEY,
    observed_at TEXT NOT NULL,
    state BLOB NOT NULL
) STRICT;`

/*
checkpointRetention is how many recent thesis checkpoints the store keeps.
Checkpoints recover recently admitted entries; older ones are dead weight the
database would otherwise carry forever.
*/
const checkpointRetention = 64

/*
PositionStore persists the stoploss attached to each open position.
*/
type PositionStore struct {
	database *sql.DB
}

/*
NewPositionStore opens the SQLite database used by position recovery.
*/
func NewPositionStore(path string) (*PositionStore, error) {
	if path == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: path required",
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

	store := &PositionStore{database: database}

	if err := store.EnsureSchema(); err != nil {
		_ = database.Close()
		return nil, err
	}

	return store, nil
}

/*
EnsureSchema creates the required tables if they do not already exist.
*/
func (store *PositionStore) EnsureSchema() error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	if _, err := store.database.Exec(positionStoplossSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: schema failed",
			err,
		))
	}

	if _, err := store.database.Exec(thesisCheckpointSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: thesis schema failed",
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
SaveThesis appends one complete pre-execution thesis checkpoint.
*/
func (store *PositionStore) SaveThesis(thesis *types.Thesis) error {
	if store == nil || store.database == nil {
		return fmt.Errorf("position store: database required")
	}

	state, err := thesis.MarshalState()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: marshal thesis failed [%s]", err.Error()),
			err,
		))
	}

	if err := store.saveThesisCheckpoint(thesis.At, state); err != nil {
		if isNoSuchTable(err) {
			if schemaErr := store.EnsureSchema(); schemaErr != nil {
				return schemaErr
			}

			if retryErr := store.saveThesisCheckpoint(thesis.At, state); retryErr != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("position store: save thesis failed [%s]", retryErr.Error()),
					retryErr,
				))
			}

			return nil
		}

		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: save thesis failed [%s]", err.Error()),
			err,
		))
	}

	return nil
}

func (store *PositionStore) saveThesisCheckpoint(at time.Time, state []byte) error {
	if _, err := store.database.Exec(`
INSERT INTO thesis_checkpoints (observed_at, state) VALUES (?, ?)`,
		at.UTC().Format(time.RFC3339Nano), state,
	); err != nil {
		return err
	}

	// Checkpoints exist for recovery of recently admitted entries; keeping
	// every one ever written grows the database without bound.
	if _, err := store.database.Exec(`
DELETE FROM thesis_checkpoints
WHERE id <= (SELECT id FROM thesis_checkpoints ORDER BY id DESC LIMIT 1 OFFSET ?)`,
		checkpointRetention,
	); err != nil {
		return err
	}

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

	if err := store.saveStoploss(stoploss.Symbol, entryAt, state); err != nil {
		if isNoSuchTable(err) {
			if schemaErr := store.EnsureSchema(); schemaErr != nil {
				return schemaErr
			}

			if retryErr := store.saveStoploss(stoploss.Symbol, entryAt, state); retryErr != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("position store: save stoploss failed [%s]", retryErr.Error()),
					retryErr,
				))
			}

			return nil
		}

		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: save stoploss failed [%s]", err.Error()),
			err,
		))
	}

	return nil
}

func (store *PositionStore) saveStoploss(symbol string, entryAt string, state []byte) error {
	_, err := store.database.Exec(`
INSERT INTO position_stoplosses (symbol, entry_at, state) VALUES (?, ?, ?)
ON CONFLICT(symbol, entry_at) DO UPDATE SET state = excluded.state`,
		symbol,
		entryAt,
		state,
	)

	return err
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

	var state []byte
	err := store.database.QueryRow(
		"SELECT state FROM position_stoplosses WHERE symbol = ? AND entry_at = ?",
		symbol,
		entryAt.UTC().Format(time.RFC3339Nano),
	).Scan(&state)

	if isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return nil, schemaErr
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

	if _, err := store.database.Exec(
		"DELETE FROM position_stoplosses WHERE symbol = ?", symbol,
	); err != nil {
		if isNoSuchTable(err) {
			if schemaErr := store.EnsureSchema(); schemaErr != nil {
				return schemaErr
			}

			return nil
		}

		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: delete stoploss failed for %s [%s]", symbol, err.Error()),
			err,
		))
	}

	return nil
}

/*
Close releases the SQLite database.
*/
func (store *PositionStore) Close() error {
	if store == nil || store.database == nil {
		return nil
	}

	return store.database.Close()
}

func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "no such table")
}
