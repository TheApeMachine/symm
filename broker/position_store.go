package broker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	database, err := sql.Open("sqlite3", path)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: open failed for %s [%s]", path, err.Error()),
			err,
		))
	}

	database.SetMaxOpenConns(1)

	if _, err := database.Exec(positionStoplossSchema); err != nil {
		_ = database.Close()
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"position store: schema failed",
			err,
		))
	}

	if _, err := database.Exec(thesisCheckpointSchema); err != nil {
		_ = database.Close()
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"position store: thesis schema failed",
			err,
		))
	}

	return &PositionStore{database: database}, nil
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

	if _, err = store.database.Exec(`
INSERT INTO thesis_checkpoints (observed_at, state) VALUES (?, ?)`,
		thesis.At.UTC().Format(time.RFC3339Nano), state,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: save thesis failed [%s]", err.Error()),
			err,
		))
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

	_, err = store.database.Exec(`
INSERT INTO position_stoplosses (symbol, state) VALUES (?, ?)
ON CONFLICT(symbol) DO UPDATE SET state = excluded.state`,
		stoploss.Symbol,
		state,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: save stoploss failed [%s]", err.Error()),
			err,
		))
	}

	return nil
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
			"position store: database and symbol required",
			nil,
		))
	}

	var state []byte
	err := store.database.QueryRow(
		"SELECT state FROM position_stoplosses WHERE symbol = ?",
		symbol,
	).Scan(&state)

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
			"position store: database and symbol required",
			nil,
		))
	}

	if _, err := store.database.Exec(
		"DELETE FROM position_stoplosses WHERE symbol = ?", symbol,
	); err != nil {
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
