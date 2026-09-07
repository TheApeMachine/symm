package position

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
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

const openPositionSchema = `
CREATE TABLE IF NOT EXISTS open_positions (
	symbol TEXT NOT NULL,
	entry_at TEXT NOT NULL,
	state BLOB NOT NULL,
	PRIMARY KEY (symbol, entry_at)
) STRICT;
`

type storeOperation struct {
	key         string
	query       string
	args        []any
	description string
	fence       chan error
}

/*
Store provides durable, asynchronous persistence for open position facts.
The execution hot path writes transition facts asynchronously without waiting
on SQLite disk I/O, while backpressure prevents memory exhaustion and uncommitted
state loss.
*/
type Store struct {
	database  *sql.DB
	queue     chan storeOperation
	done      chan struct{}
	failed    chan struct{}
	batchSize int

	stateMu   sync.RWMutex
	closed    bool
	closeOnce sync.Once

	errorMu sync.RWMutex
	err     error
}

func NewStore(
	path string,
	queueDepth int,
	batchSize int,
) (*Store, error) {
	if queueDepth <= 0 || batchSize <= 0 || batchSize > queueDepth {
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

	database, err := sql.Open(
		"sqlite3",
		path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL",
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: open failed for %s [%s]", path, err.Error()),
			err,
		))
	}

	database.SetMaxOpenConns(1)

	store := &Store{
		database:  database,
		queue:     make(chan storeOperation, queueDepth),
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

/* EnsureSchema creates the required tables if they do not already exist. */
func (store *Store) EnsureSchema() error {
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

	return nil
}

/*
Save records one open lot's durable entry facts. Only a filled lot has them,
so a holding without valid entry facts is refused.
*/
func (store *Store) Save(holding *types.Holding) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	if holding == nil || holding.EntryAt == nil || holding.EntryAt.IsZero() || holding.EntryPrice == nil || holding.Qty == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: valid holding entry facts required to save an open position",
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

	return store.enqueue(storeOperation{
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
when none exists.
*/
func (store *Store) Load(
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
	err := store.database.QueryRowContext(
		ctx,
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

	return holding, nil
}

/*
Delete removes every stored open lot for a symbol after its position closes.
*/
func (store *Store) Delete(symbol string) error {
	if store == nil || store.database == nil || symbol == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: database required",
			nil,
		))
	}

	return store.enqueue(storeOperation{
		key:         symbol,
		query:       "DELETE FROM open_positions WHERE symbol = ?",
		args:        []any{symbol},
		description: "delete open positions for " + symbol,
	})
}

/*
Sync waits until every position write accepted before its fence is durable.
*/
func (store *Store) Sync() error {
	if store == nil {
		return nil
	}

	fence := make(chan error, 1)

	if err := store.enqueue(storeOperation{fence: fence}); err != nil {
		return err
	}

	return <-fence
}

/* Failed returns a notification channel closed when asynchronous persistence fails. */
func (store *Store) Failed() <-chan struct{} {
	if store == nil {
		return nil
	}

	return store.failed
}

/* Error returns the persistent error causing store failure, if any. */
func (store *Store) Error() error {
	if store == nil {
		return nil
	}

	store.errorMu.RLock()
	defer store.errorMu.RUnlock()

	return store.err
}

func (store *Store) setError(err error) {
	store.errorMu.Lock()
	defer store.errorMu.Unlock()

	if store.err == nil {
		store.err = err
	}
}

/* Close releases the writer worker and closes the database connection. */
func (store *Store) Close() error {
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

func (store *Store) enqueue(operation storeOperation) error {
	store.stateMu.RLock()
	defer store.stateMu.RUnlock()

	if store.closed {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"position store: writer is closed",
			store.Error(),
		))
	}

	if err := store.Error(); err != nil {
		return err
	}

	select {
	case store.queue <- operation:
		return nil
	case <-store.failed:
		return store.Error()
	}
}

func (store *Store) runWriter() {
	defer close(store.done)

	batch := make([]storeOperation, 0, store.batchSize)

	for operation := range store.queue {
		batch = append(batch[:0], operation)
		draining := true

		for draining && len(batch) < store.batchSize && batch[len(batch)-1].fence == nil {
			select {
			case next, open := <-store.queue:
				if !open {
					if err := store.persist(batch); err != nil {
						store.fail(err, batch)
					}

					return
				}

				batch = append(batch, next)
			default:
				draining = false
			}
		}

		if err := store.persist(batch); err != nil {
			store.fail(err, batch)
			return
		}
	}
}

func (store *Store) persist(operations []storeOperation) error {
	transaction, err := store.database.Begin()

	if err != nil {
		return err
	}

	for _, operation := range operations {
		if operation.query == "" {
			continue
		}

		if _, err := transaction.Exec(operation.query, operation.args...); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("position store: %s: %w", operation.description, err)
		}
	}

	if err := transaction.Commit(); err != nil {
		return err
	}

	for _, operation := range operations {
		if operation.fence != nil {
			operation.fence <- nil
		}
	}

	return nil
}

func (store *Store) fail(err error, operations []storeOperation) {
	wrapped := errnie.Error(errnie.Err(
		errnie.IO,
		"position store: asynchronous persistence failed",
		err,
	))
	store.setError(wrapped)
	close(store.failed)
	notifyFences(operations, wrapped)

	for operation := range store.queue {
		notifyFences([]storeOperation{operation}, wrapped)
	}
}

func notifyFences(operations []storeOperation, err error) {
	for _, operation := range operations {
		if operation.fence != nil {
			operation.fence <- err
		}
	}
}

func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "no such table")
}
