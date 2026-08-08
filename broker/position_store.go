package broker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/theapemachine/symm/types"
)

const positionStoplossSchema = `
CREATE TABLE IF NOT EXISTS position_stoplosses (
    symbol TEXT PRIMARY KEY,
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
		return nil, fmt.Errorf("position store: path required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("position store: create directory: %w", err)
	}

	database, err := sql.Open("sqlite3", path)

	if err != nil {
		return nil, fmt.Errorf("position store: open: %w", err)
	}

	database.SetMaxOpenConns(1)

	if _, err := database.Exec(positionStoplossSchema); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("position store: schema: %w", err)
	}

	return &PositionStore{database: database}, nil
}

/*
Save stores the current stoploss state for its position symbol.
*/
func (store *PositionStore) Save(stoploss *types.Stoploss) error {
	if store == nil || store.database == nil {
		return fmt.Errorf("position store: database required")
	}

	state, err := stoploss.MarshalState()

	if err != nil {
		return err
	}

	_, err = store.database.Exec(`
INSERT INTO position_stoplosses (symbol, state) VALUES (?, ?)
ON CONFLICT(symbol) DO UPDATE SET state = excluded.state`,
		stoploss.Symbol,
		state,
	)

	if err != nil {
		return fmt.Errorf("position store: save %s: %w", stoploss.Symbol, err)
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
		return nil, fmt.Errorf("position store: database and symbol required")
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
		return nil, fmt.Errorf("position store: load %s: %w", symbol, err)
	}

	return types.RestoreStoploss(ctx, state)
}

/*
Delete removes the stoploss after its position closes.
*/
func (store *PositionStore) Delete(symbol string) error {
	if store == nil || store.database == nil || symbol == "" {
		return fmt.Errorf("position store: database and symbol required")
	}

	if _, err := store.database.Exec(
		"DELETE FROM position_stoplosses WHERE symbol = ?", symbol,
	); err != nil {
		return fmt.Errorf("position store: delete %s: %w", symbol, err)
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
