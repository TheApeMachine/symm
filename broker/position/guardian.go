package position

import (
	"context"
	"sync"
	"sync/atomic"

	disruptor "github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

// guardianCapacity is the preallocated memory bound of each lot's priority ring.
// The Disruptor representation requires a power of two.
const guardianCapacity uint32 = 8192
const guardianCapacityMask = int64(guardianCapacity) - 1

// Guardian owns a lot's serial event processing and transport lifecycle.
// Publication waits only for another local publication, never for a consumer.
// Close prevents further publication and lets accepted events drain; Done closes
// when the listener exits, including when Close runs inside an event handler.
type Guardian struct {
	position    *Regulator
	ring        disruptor.Disruptor
	slots       [guardianCapacity]any
	publication sync.Mutex
	started     bool
	closed      bool
	stopContext func() bool
	Done        chan struct{}
	Watermark   atomic.Uint64
}

// NewGuardian constructs the single-writer ring used under publication.
func NewGuardian(position *Regulator) *Guardian {
	guardian := &Guardian{position: position, Done: make(chan struct{})}
	ring, err := disruptor.New(
		disruptor.Options.BufferCapacity(guardianCapacity),
		disruptor.Options.NewHandlerGroup(guardian),
	)

	if err != nil {
		// These are static representation invariants, not runtime input.
		panic(errnie.Error(errnie.Err(errnie.Internal, "guardian: invalid ring configuration", err)))
	}
	guardian.ring = ring
	return guardian
}

// Start admits publication and ties the listener's lifetime to its parent.
func (guardian *Guardian) Start(ctx context.Context) {
	guardian.publication.Lock()
	defer guardian.publication.Unlock()

	if guardian.started || guardian.closed {
		panic("guardian: start requires an unstarted, open listener")
	}
	guardian.started = true
	guardian.stopContext = context.AfterFunc(ctx, func() {
		if err := guardian.Close(); err != nil {
			errnie.Error(err)
		}
	})
	go func() {
		guardian.ring.Listen()
		close(guardian.Done)
	}()
}

// Publish reserves once and reports capacity exhaustion without waiting for the
// handler. The caller retains ownership of a refused event and receives an error.
func (guardian *Guardian) Publish(value any) error {
	if guardian == nil {
		return errnie.Error(errnie.Err(errnie.NotFound, "guardian: transport unavailable", nil))
	}
	guardian.publication.Lock()
	defer guardian.publication.Unlock()

	if !guardian.started || guardian.closed {
		return errnie.Error(errnie.Err(errnie.Conflict, "guardian: listener is not accepting events", nil))
	}
	sequence := guardian.ring.TryReserve(1)

	if sequence < 0 {
		return errnie.Error(errnie.Err(errnie.NotAcceptable, "guardian: priority ring saturated", nil))
	}
	guardian.slots[sequence&guardianCapacityMask] = value
	guardian.ring.Commit(sequence, sequence)
	return nil
}

// Handle processes committed slots in order and releases their retained payloads.
func (guardian *Guardian) Handle(lower, upper int64) {
	for sequence := lower; sequence <= upper; sequence++ {
		slot := &guardian.slots[sequence&guardianCapacityMask]
		guardian.dispatch(*slot)
		*slot = nil
		guardian.Watermark.Add(1)
	}
}

// Close fences publishers before requesting a drain. It never joins its own
// listener; shutdown callers can wait on Done after asking every owner to close.
func (guardian *Guardian) Close() error {
	guardian.publication.Lock()
	defer guardian.publication.Unlock()

	if guardian.closed {
		return nil
	}
	guardian.closed = true

	if guardian.stopContext != nil {
		guardian.stopContext()
	}

	if !guardian.started {
		close(guardian.Done)
	}
	return errnie.Error(guardian.ring.Close())
}

func (guardian *Guardian) dispatch(value any) {
	var err error

	switch payload := value.(type) {
	case kraken.ExecutionData:
		err = guardian.position.Apply(payload)
	case kraken.Level3Data:
		err = guardian.position.Mark(payload.Timestamp)
	case func() error:
		err = payload()
	default:
		err = errnie.Err(errnie.Validation, "guardian: unsupported event", nil)
	}

	if err != nil {
		errnie.Error(err)
	}
}
