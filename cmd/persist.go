package cmd

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	nmruntime "github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
)

const (
	persistWorkers = 2
	persistQueue   = 64
)

/*
tickPersist writes the finished envelope after the model ring. Ingress must not
marshal that envelope — Step hands it to the disruptor asynchronously.
*/
type tickPersist struct {
	*nmruntime.System
	store  *store.Client
	armed  atomic.Bool
	queue  chan *nmruntime.Envelope
	wg     sync.WaitGroup
	closeQ sync.Once
}

func newTickPersist(ctx context.Context, client *store.Client) *tickPersist {
	persist := &tickPersist{
		store: client,
		queue: make(chan *nmruntime.Envelope, persistQueue),
	}
	persist.System = nmruntime.NewSystem(ctx, "tick-persist", persist)
	persist.Transition(nmruntime.READY)

	return persist
}

func (persist *tickPersist) Arm() {
	if !persist.armed.CompareAndSwap(false, true) {
		return
	}

	for range persistWorkers {
		persist.wg.Add(1)
		go persist.worker()
	}
}

/*
Disarm stops accepting new tick writes and closes the queue so workers drain.
*/
func (persist *tickPersist) Disarm() {
	persist.armed.Store(false)
	persist.closeQ.Do(func() {
		close(persist.queue)
	})
}

/*
Wait joins persist workers after Disarm has closed the queue.
*/
func (persist *tickPersist) Wait() {
	persist.wg.Wait()
}

func (persist *tickPersist) Step(envelope *nmruntime.Envelope) *nmruntime.Envelope {
	if envelope == nil || persist.store == nil || !persist.armed.Load() {
		return envelope
	}

	select {
	case persist.queue <- envelope:
	default:
		// Coalesce under backpressure: drop the oldest queued tick, then try
		// once more; if still full, drop this envelope.
		select {
		case dropped := <-persist.queue:
			errnie.Warn(fmt.Sprintf(
				"[tick-persist] queue full — dropped older tick n=%d", dropped.N,
			))
		default:
		}

		select {
		case persist.queue <- envelope:
		default:
			errnie.Warn(fmt.Sprintf(
				"[tick-persist] queue full — dropped tick n=%d", envelope.N,
			))
		}
	}

	return envelope
}

func (persist *tickPersist) worker() {
	defer persist.wg.Done()

	for envelope := range persist.queue {
		persist.write(envelope)
	}
}

func (persist *tickPersist) write(envelope *nmruntime.Envelope) {
	payload, err := nmruntime.Sonic.Marshal(envelope)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[tick-persist] marshal failed",
			err,
		).With("n", envelope.N))

		return
	}

	key := fmt.Sprintf(
		"workspace/tick/aggregate/%019d/%019d",
		data.Epoch.Load(),
		envelope.N,
	)

	if _, err := persist.store.WriteAtPrefix(key, payload); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[tick-persist] write failed",
			err,
		).With("n", envelope.N))
	}
}
