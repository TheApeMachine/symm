package broker

import (
	"fmt"
	"sync/atomic"

	"github.com/theapemachine/errnie"
)

type positionStoreOperation struct {
	key         string
	query       string
	args        []any
	description string
	fence       chan error
}

func (store *PositionStore) enqueue(operation positionStoreOperation) error {
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

	if operation.fence != nil {
		select {
		case store.queue <- operation:
			return nil
		case <-store.failed:
			return store.Error()
		}
	}

	select {
	case store.queue <- operation:
		return nil
	case <-store.failed:
		return store.Error()
	default:
		atomic.AddUint64(&store.shedCount, 1)

		errnie.Warn(fmt.Sprintf(
			"position store: write queue full, shedding %s",
			operation.description,
		))

		return nil
	}
}

/*
Sync waits until every position write accepted before its fence is durable.
Read paths use it to preserve read-after-write behavior without putting SQLite
back onto market and guardian paths.
*/
func (store *PositionStore) Sync() error {
	if store == nil {
		return nil
	}

	fence := make(chan error, 1)

	if err := store.enqueue(positionStoreOperation{fence: fence}); err != nil {
		return err
	}

	return <-fence
}

func (store *PositionStore) Failed() <-chan struct{} {
	if store == nil {
		return nil
	}

	return store.failed
}

func (store *PositionStore) Error() error {
	if store == nil {
		return nil
	}

	store.errorMu.RLock()
	defer store.errorMu.RUnlock()

	return store.err
}

func (store *PositionStore) setError(err error) {
	store.errorMu.Lock()
	defer store.errorMu.Unlock()

	if store.err == nil {
		store.err = err
	}
}

func (store *PositionStore) runWriter() {
	defer close(store.done)

	batch := make([]positionStoreOperation, 0, store.batchSize)

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

func (store *PositionStore) persist(operations []positionStoreOperation) error {
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

func (store *PositionStore) fail(err error, operations []positionStoreOperation) {
	wrapped := errnie.Error(errnie.Err(
		errnie.IO,
		"position store: asynchronous persistence failed",
		err,
	))
	store.setError(wrapped)
	close(store.failed)
	notifyPositionFences(operations, wrapped)

	for operation := range store.queue {
		notifyPositionFences([]positionStoreOperation{operation}, wrapped)
	}
}

func notifyPositionFences(operations []positionStoreOperation, err error) {
	for _, operation := range operations {
		if operation.fence != nil {
			operation.fence <- err
		}
	}
}
