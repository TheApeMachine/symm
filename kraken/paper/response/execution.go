package response

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
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
	pool      *qpool.Q[any]
	isActive  atomic.Bool
	model     []map[string]any
	observers []types.Socket
}

func NewExecutions(ctx context.Context, pool *qpool.Q[any]) *Executions {
	ctx, cancel := context.WithCancel(ctx)

	return &Executions{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		model:     make([]map[string]any, 0),
		observers: make([]types.Socket, 0),
	}
}

func (executions *Executions) Send(message []byte) *types.SocketMessage {
	var in *types.SocketMessage

	if err := sonic.Unmarshal(message, &in); err != nil {
		return nil
	}

	incoming := make(map[string]map[string]any)

	if err := sonic.Unmarshal(in.Data, &incoming); err != nil {
		return nil
	}

	switch in.Method {
	case "subscribe":
		executions.isActive.Store(true)
	case "unsubscribe":
		executions.isActive.Store(false)
	case "add_order":
		for _, execution := range incoming {
			executions.model = append(executions.model, execution)
		}
	case "cancel_order":
		for _, execution := range incoming {
			orderID, _ := execution["order_id"].(string)

			for index, stored := range executions.model {
				storedID, _ := stored["order_id"].(string)

				if storedID != orderID {
					continue
				}

				executions.model = slices.Delete(executions.model, index, 1)
				break
			}
		}
	}

	out := &types.SocketMessage{
		Channel: "executions",
		Success: true,
		Data:    in.Data,
	}

	for _, socket := range executions.observers {
		socket.Send(in.Data)
	}

	return out
}

func (executions *Executions) PublishFill(execution map[string]any) {
	executions.model = append(executions.model, execution)

	execID, _ := execution["exec_id"].(string)

	data, err := sonic.Marshal(map[string]map[string]any{
		execID: execution,
	})

	if err != nil {
		return
	}

	for _, socket := range executions.observers {
		socket.Send(data)
	}
}

func (executions *Executions) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		executions.observers = append(executions.observers, socket)
	}
}
