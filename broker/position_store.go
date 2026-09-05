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

/*
openPositionSchema records the durable entry facts of an open lot, keyed by the
lot it belongs to. It is deliberately not the old protection table: stop
geometry is gone, and a row of it could never be attributed to a lot the agent
now manages itself, so those rows are left where they are rather than adopted.
*/
const openPositionSchema = `
CREATE TABLE IF NOT EXISTS open_positions (
    symbol TEXT NOT NULL,
    entry_at TEXT NOT NULL,
    state BLOB NOT NULL,
    PRIMARY KEY (symbol, entry_at)
) STRICT;`

/*
PositionStore persists the entry facts of each open position, so a restart can
re-adopt a lot it still owns at the venue rather than rediscovering it with no
basis.
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

	if _, err := store.database.Exec(openPositionSchema); err != nil {
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
Save records one open lot's durable entry facts. Only a filled lot has them,
so a holding with no entry time is refused rather than written as a position
that never opened.
*/
func (store *PositionStore) Save(holding *types.Holding) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	if holding == nil || holding.EntryAt == nil || holding.EntryAt.IsZero() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: holding entry time required to save an open position",
			nil,
		))
	}

	state, err := json.Marshal(holding)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: marshal holding failed [%s]", err.Error()),
			err,
		))
	}

	entryAt := holding.EntryAt.UTC().Format(time.RFC3339Nano)

	return store.enqueue(positionStoreOperation{
		key: holding.Symbol,
		query: `
INSERT INTO open_positions (symbol, entry_at, state) VALUES (?, ?, ?)
ON CONFLICT(symbol, entry_at) DO UPDATE SET state = excluded.state`,
		args:        []any{holding.Symbol, entryAt, state},
		description: "save open position for " + holding.Symbol,
	})
}

/*
Load returns the open lot stored for a symbol at the given entry time, or nil
when none exists. A row from a different entry time on the same symbol — an
already-closed trade's leftover state — is never returned: recovery must not
mistake one lot's basis for another's.
*/
func (store *PositionStore) Load(
	ctx context.Context,
	symbol string,
	entryAt time.Time,
) (*types.Holding, error) {
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
		"SELECT state FROM open_positions WHERE symbol = ? AND entry_at = ?",
		symbol,
		entryAt.UTC().Format(time.RFC3339Nano),
	).Scan(&state)

	if isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"position store: load open position failed for %s [%s]",
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
			fmt.Sprintf("position store: load open position failed for %s [%s]", symbol, err.Error()),
			err,
		))
	}

	holding := &types.Holding{}

	if err := json.Unmarshal(state, holding); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: unmarshal holding failed for %s", symbol),
			err,
		))
	}

	_ = ctx

	return holding, nil
}

/*
Delete removes every stored open lot for a symbol after its position closes.
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
		query:       "DELETE FROM open_positions WHERE symbol = ?",
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

/*
AdoptLegacyStore imports position state left behind in a separate database
file.

Position state used to live in its own SQLite file alongside the capture. It
belongs with the capture: a position is part of the record of what a run did,
and keeping it apart meant the trade journal and the stoploss recovery state
could not be read in the same transaction as the lifecycle tape that explains
them. This carries the old file's rows into the current store once, so
consolidating does not discard the history that was already recorded.

The import is idempotent. Trades are unique by decision, stoploss state by
symbol and entry instant, so a row already present is left exactly as it is
rather than being overwritten by an older copy of itself. A missing legacy
file is not an error: it is the ordinary state after the first consolidation.
*/
func (store *PositionStore) AdoptLegacyStore(path string) (int64, error) {
	if store == nil || store.database == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	if path == "" {
		return 0, nil
	}

	if _, err := os.Stat(path); err != nil {
		return 0, nil
	}

	if _, err := store.database.Exec(`ATTACH DATABASE ? AS legacy`, path); err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: attach legacy store %s [%s]", path, err.Error()),
			err,
		))
	}

	defer func() {
		_, _ = store.database.Exec(`DETACH DATABASE legacy`)
	}()

	adopted := int64(0)

	for _, statement := range []string{
		`INSERT OR IGNORE INTO main.position_trades (
		     symbol, status, decision_id, entry_at, entry_price, entry_fee, qty,
		     exit_at, exit_price, exit_fee, pnl, return_pct, trigger_reason,
		     trigger_mark, floor, peak, profit_line, locked, cause,
		     raw_position, created_at, updated_at
		 )
		 SELECT symbol, status, decision_id, entry_at, entry_price, entry_fee, qty,
		        exit_at, exit_price, exit_fee, pnl, return_pct, trigger_reason,
		        trigger_mark, floor, peak, profit_line, locked, cause,
		        raw_position, created_at, updated_at
		 FROM legacy.position_trades`,
	} {
		result, err := store.database.Exec(statement)

		if err != nil {
			return adopted, errnie.Error(errnie.Err(
				errnie.IO,
				"position store: adopt legacy rows",
				err,
			))
		}

		affected, err := result.RowsAffected()

		if err == nil {
			adopted += affected
		}
	}

	return adopted, nil
}
