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

/*
tickPersist writes the finished envelope after the model ring. Ingress must not
marshal that envelope — Step hands it to the disruptor asynchronously.
*/
type tickPersist struct {
	*nmruntime.System
	store *store.Client
	armed atomic.Bool
	wg    sync.WaitGroup
}

func newTickPersist(ctx context.Context, client *store.Client) *tickPersist {
	persist := &tickPersist{store: client}
	persist.System = nmruntime.NewSystem(ctx, "tick-persist", persist)
	persist.Transition(nmruntime.READY)

	return persist
}

func (persist *tickPersist) Arm() {
	persist.armed.Store(true)
}

/*
Disarm stops accepting new tick writes during shutdown.
*/
func (persist *tickPersist) Disarm() {
	persist.armed.Store(false)
}

/*
Wait blocks until every in-flight write goroutine has finished.
*/
func (persist *tickPersist) Wait() {
	persist.wg.Wait()
}

func (persist *tickPersist) Step(envelope *nmruntime.Envelope) *nmruntime.Envelope {
	if envelope == nil || persist.store == nil || !persist.armed.Load() {
		return envelope
	}

	// Pipeline rings are done with this envelope. Marshal off-thread so JSON/S3
	// never stalls the disruptor (that freeze left impulse/kernels on last paint).
	persist.wg.Add(1)

	go func() {
		defer persist.wg.Done()
		persist.write(envelope)
	}()

	return envelope
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
