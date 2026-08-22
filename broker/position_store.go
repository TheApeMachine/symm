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
    symbol TEXT PRIMARY KEY,
    state BLOB NOT NULL
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
Save stores the current stoploss state for its position symbol.
*/
func (store *PositionStore) Save(stoploss *types.Stoploss) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
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

	if err := store.saveStoploss(stoploss.Symbol, state); err != nil {
		if isNoSuchTable(err) {
			if schemaErr := store.EnsureSchema(); schemaErr != nil {
				return schemaErr
			}

			if retryErr := store.saveStoploss(stoploss.Symbol, state); retryErr != nil {
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

func (store *PositionStore) saveStoploss(symbol string, state []byte) error {
	_, err := store.database.Exec(`
INSERT INTO position_stoplosses (symbol, state) VALUES (?, ?)
ON CONFLICT(symbol) DO UPDATE SET state = excluded.state`,
		symbol,
		state,
	)

	return err
}

/*
Load returns the stored stoploss for a symbol, or nil when none exists.
*/
func (store *PositionStore) Load(
	ctx context.Context,
	symbol string,
) (*types.Stoploss, error) {
	if store == nil || store.database == nil || symbol == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	var state []byte
	err := store.database.QueryRow(
		"SELECT state FROM position_stoplosses WHERE symbol = ?",
		symbol,
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
Delete removes the stoploss after its position closes.
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
