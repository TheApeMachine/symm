package store

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

/*
AsyncWitnessWriter decouples Hindsight artifact persistence from the Disruptor
hot path. witnessNode hands it fully-formed witnesses (including the already
encoded payload bytes) and returns immediately; a single background worker
drains the queue and persists them to the repository in batches, so a slow
SQLite write never blocks the consumer thread that is processing market frames.

Enqueue never fails and never allocates on the steady path: a full queue drops
the witness and records the drop count so an operator can see persistence is
falling behind, mirroring the overflow policy the audit recorder owns rather
than silently blocking the pipeline.
*/
type AsyncWitnessWriter struct {
	repository Repository

	queue  chan hindsight.ArtifactWitness
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once

	droppedMu sync.Mutex
	dropped   uint64
}

/*
NewAsyncWitnessWriter wires a bounded witness queue over an already-open
repository. The queue depth and flush interval are explicit capacity policy:
each enqueued witness may carry a ~66KB state payload, so the depth bounds
resident memory while the worker coalesces inserts into one transaction.
*/
func NewAsyncWitnessWriter(
	ctx context.Context,
	repository Repository,
	queueDepth int,
	flushInterval time.Duration,
) *AsyncWitnessWriter {
	if queueDepth < 1 {
		queueDepth = 1024
	}

	if flushInterval <= 0 {
		flushInterval = 50 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(ctx)

	writer := &AsyncWitnessWriter{
		repository: repository,
		queue:      make(chan hindsight.ArtifactWitness, queueDepth),
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
	}

	go writer.run(flushInterval)

	return writer
}

/*
Enqueue hands one witness to the background worker without blocking on
persistence. It is safe to call from any goroutine.
*/
func (writer *AsyncWitnessWriter) Enqueue(witness hindsight.ArtifactWitness) {
	if writer == nil || writer.repository == nil {
		return
	}

	select {
	case writer.queue <- witness:
	default:
		writer.droppedMu.Lock()
		writer.dropped++
		writer.droppedMu.Unlock()
	}
}

/*
Dropped reports how many witnesses were discarded because the queue was full.
A non-zero value means witness persistence fell behind the ingress rate; it is
observability, never silent data loss that would otherwise stall trading.
*/
func (writer *AsyncWitnessWriter) Dropped() uint64 {
	if writer == nil {
		return 0
	}

	writer.droppedMu.Lock()
	defer writer.droppedMu.Unlock()

	return writer.dropped
}

/*
Close stops the background worker after draining the queue, then returns.
*/
func (writer *AsyncWitnessWriter) Close() error {
	if writer == nil {
		return nil
	}

	writer.once.Do(func() {
		writer.cancel()
		<-writer.done
	})

	return nil
}

func (writer *AsyncWitnessWriter) run(flushInterval time.Duration) {
	defer close(writer.done)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	pending := make([]hindsight.ArtifactWitness, 0, 256)

	flush := func() error {
		if len(pending) == 0 {
			return nil
		}

		batch := pending
		pending = make([]hindsight.ArtifactWitness, 0, 256)

		// The SQLite repository exposes a batched transactional write; every
		// other repository falls back to per-record writes through the
		// narrow Repository contract.
		if batched, supported := writer.repository.(interface {
			WriteWitnesses([]hindsight.ArtifactWitness) error
		}); supported {
			if err := batched.WriteWitnesses(batch); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"store: async artifact witness batch persistence failed",
					err,
				))

				return err
			}

			return nil
		}

		for _, witness := range batch {
			if err := writer.repository.WriteWitness(witness); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"store: async artifact witness persistence failed",
					err,
				))

				return err
			}
		}

		return nil
	}

	for {
		select {
		case <-writer.ctx.Done():
			// Drain everything already enqueued before the final flush so
			// Close never strands witnesses that arrived after the last
			// batch boundary.
			for {
				select {
				case witness := <-writer.queue:
					pending = append(pending, witness)
				default:
					_ = flush()
					return
				}
			}
		case witness := <-writer.queue:
			pending = append(pending, witness)

			// Coalesce everything already queued into this batch so a burst
			// of frames compresses into fewer repository round-trips.
			for drain := true; drain && len(pending) < cap(pending); {
				select {
				case next := <-writer.queue:
					pending = append(pending, next)
				default:
					drain = false
				}
			}

			if err := flush(); err != nil {
				return
			}
		case <-ticker.C:
			if err := flush(); err != nil {
				return
			}
		}
	}
}
