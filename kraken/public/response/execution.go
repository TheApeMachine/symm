package response

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/types"
)

/*
Executions simulates the Kraken executions channel and publishes the same raw
frames and derived envelopes as the live private websocket.
*/
type Executions struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	isActive  atomic.Bool
	model     *sync.Map
	observers *sync.Map
}

func NewExecutions(ctx context.Context) *Executions {
	ctx, cancel := context.WithCancel(ctx)

	return &Executions{
		ctx:       ctx,
		cancel:    cancel,
		model:     &sync.Map{},
		observers: &sync.Map{},
	}
}

func (executions *Executions) Send(artifact *datura.Artifact) *datura.Artifact {
	method := datura.Peek[string](artifact, "method")

	switch method {
	case "subscribe":
		executions.isActive.Store(true)
	case "unsubscribe":
		executions.isActive.Store(false)
	}

	executions.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact)
		return true
	})

	return nil
}

func (executions *Executions) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		executions.observers.Store(uuid.NewString(), socket)
	}
}
