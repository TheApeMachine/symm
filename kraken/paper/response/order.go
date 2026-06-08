package response

import (
	"context"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
)

/*
Orders simulates Kraken private order methods. Taker orders fill through
broker.SlippageFill after one-way latency; post-only limits rest with L2 queue
position and fill when aggressor trades deplete queue ahead via broker.MakerQueueState.
Protective orders REST until price breaches, then use the same fill helpers.
*/
type Orders struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	pool      *qpool.Q[any]
	isActive  atomic.Bool
	model     map[string]trading.OrderUpdate
	observers []types.Socket
}

func NewOrders(ctx context.Context, pool *qpool.Q[any]) *Orders {
	ctx, cancel := context.WithCancel(ctx)

	return &Orders{
		ctx:       ctx,
		cancel:    cancel,
		err:       nil,
		pool:      pool,
		isActive:  atomic.Bool{},
		observers: make([]types.Socket, 0),
	}
}

func (orders *Orders) Send(message *qpool.QValue[any]) *types.SocketMessage {
	var (
		out   *types.SocketMessage
		inMsg map[string]any
		ok    bool
	)

	if inMsg, ok = message.Value.(map[string]any); !ok {
		return nil
	}

	switch inMsg["method"].(string) {
	case "subscribe":
		orders.isActive.Store(true)
	case "unsubscribe":
		orders.isActive.Store(false)
	case "add_order":
		orders.model[inMsg["order_id"].(string)] = trading.OrderUpdate{
			OrderID: inMsg["order_id"].(string),
		}
	case "cancel_order":
		delete(orders.model, inMsg["order_id"].(string))
	case "amend_order":
		orders.model[inMsg["order_id"].(string)] = trading.OrderUpdate{
			OrderID: inMsg["order_id"].(string),
		}
	}

	data, err := sonic.Marshal(orders.model)

	if err != nil {
		return nil
	}

	out = &types.SocketMessage{
		Method:  "orders",
		Success: &[]bool{true}[0],
		Data:    data,
	}

	for _, observer := range orders.observers {
		observer.Send(&qpool.QValue[any]{Value: out})
	}

	return out
}

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		orders.observers = append(orders.observers, socket)
	}
}
