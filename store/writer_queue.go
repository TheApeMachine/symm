package store

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

type writerOperationKind uint8

const (
	writerCapture writerOperationKind = iota + 1
	writerManifest
	writerLifecycle
	writerFence
)

type writerOperation struct {
	kind        writerOperationKind
	identity    hindsight.CaptureIdentity
	endpoint    string
	captureKind string
	payload     []byte
	at          time.Time
	manifest    hindsight.EnvelopeManifest
	runID       hindsight.RunID
	lifecycle   hindsight.LifecycleEvent
	fence       chan error
}

/*
enqueue accepts one ordered persistence operation. It does not wait for the
repository, but it does apply bounded backpressure rather than discard raw
market input when storage is genuinely saturated.
*/
func (writer *Writer) enqueue(operation writerOperation) error {
	writer.stateMu.RLock()
	defer writer.stateMu.RUnlock()

	if writer.closed {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: capture writer is closed",
			writer.Error(),
		))
	}

	if err := writer.Error(); err != nil {
		return err
	}

	select {
	case writer.queue <- operation:
		return nil
	case <-writer.failed:
		return writer.Error()
	}
}

/*
Sync waits until every operation accepted before its fence has reached the
repository. It is an explicit durability boundary for shutdown and tests, not
part of the market-event hot path.
*/
func (writer *Writer) Sync() error {
	if writer == nil {
		return nil
	}

	fence := make(chan error, 1)

	if err := writer.enqueue(writerOperation{kind: writerFence, fence: fence}); err != nil {
		return err
	}

	return <-fence
}

/*
Failed closes when background persistence fails. Error returns the exact cause.
Callers should terminate the run because accepted raw input is no longer fully
durable.
*/
func (writer *Writer) Failed() <-chan struct{} {
	if writer == nil {
		return nil
	}

	return writer.failed
}

func (writer *Writer) Error() error {
	if writer == nil {
		return nil
	}

	writer.errorMu.RLock()
	defer writer.errorMu.RUnlock()

	return writer.err
}

/*
Close rejects new operations, drains every accepted operation, and waits for
the storage worker. Repository ownership remains with the caller.
*/
func (writer *Writer) Close() error {
	if writer == nil {
		return nil
	}

	writer.closeOnce.Do(func() {
		writer.stateMu.Lock()
		writer.closed = true
		close(writer.queue)
		writer.stateMu.Unlock()

		<-writer.done
	})

	return writer.Error()
}

func (writer *Writer) run() {
	defer close(writer.done)

	batch := make([]writerOperation, 0, writer.batchSize)

	for operation := range writer.queue {
		batch = append(batch[:0], operation)
		draining := true

		for draining && len(batch) < writer.batchSize && batch[len(batch)-1].kind != writerFence {
			select {
			case next, open := <-writer.queue:
				if !open {
					if err := writer.persist(batch); err != nil {
						writer.fail(err, batch)
					}

					return
				}

				batch = append(batch, next)
			default:
				draining = false
			}
		}

		if err := writer.persist(batch); err != nil {
			writer.fail(err, batch)
			return
		}
	}
}

func (writer *Writer) persist(operations []writerOperation) error {
	if writer.repository != nil {
		if repository, ok := writer.repository.(interface {
			writeOperations([]writerOperation) error
		}); ok {
			if err := repository.writeOperations(operations); err != nil {
				return err
			}
		} else {
			for _, operation := range operations {
				if err := writer.persistOne(operation); err != nil {
					return err
				}
			}
		}
	}

	for _, operation := range operations {
		if operation.fence != nil {
			operation.fence <- nil
		}
	}

	return nil
}

func (writer *Writer) persistOne(operation writerOperation) error {
	switch operation.kind {
	case writerCapture:
		return writer.repository.WriteCapture(
			operation.identity,
			operation.endpoint,
			operation.captureKind,
			operation.payload,
			operation.at,
		)
	case writerManifest:
		return writer.repository.WriteManifest(operation.manifest)
	case writerLifecycle:
		repository, ok := writer.repository.(interface {
			WriteLifecycleEvent(hindsight.RunID, hindsight.LifecycleEvent) error
		})

		if !ok {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"store: repository does not support lifecycle events",
				nil,
			))
		}

		return repository.WriteLifecycleEvent(operation.runID, operation.lifecycle)
	case writerFence:
		return nil
	default:
		return fmt.Errorf("store: unknown writer operation %d", operation.kind)
	}
}

func (writer *Writer) fail(err error, operations []writerOperation) {
	wrapped := errnie.Error(errnie.Err(
		errnie.IO,
		"store: asynchronous capture persistence failed",
		err,
	))

	writer.errorMu.Lock()
	writer.err = wrapped
	writer.errorMu.Unlock()
	close(writer.failed)

	identity := failureIdentity(operations)

	if writer.repository != nil && identity.Valid() {
		_ = writer.repository.MarkGapped(
			identity.Run,
			identity.Sequence,
			"capture_persistence_failure",
			wrapped.Error(),
		)
	}

	notifyFences(operations, wrapped)

	for operation := range writer.queue {
		notifyFences([]writerOperation{operation}, wrapped)
	}
}

func failureIdentity(operations []writerOperation) hindsight.CaptureIdentity {
	for _, operation := range operations {
		if operation.identity.Valid() {
			return operation.identity
		}

		if operation.manifest.Envelope.Origin.Valid() {
			return operation.manifest.Envelope.Origin
		}
	}

	return hindsight.CaptureIdentity{}
}

func notifyFences(operations []writerOperation, err error) {
	for _, operation := range operations {
		if operation.fence != nil {
			operation.fence <- err
		}
	}
}
