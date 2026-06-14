package response

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
)

type Orders struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	pool            *qpool.Q[any]
	isActive        atomic.Bool
	model           []trading.OrderUpdate
	observers       []types.Socket
	bookDepthLevels int
}

func NewOrders(
	ctx context.Context,
	pool *qpool.Q[any],
) *Orders {
	ctx, cancel := context.WithCancel(ctx)

	return &Orders{
		ctx:             ctx,
		cancel:          cancel,
		err:             nil,
		pool:            pool,
		model:           make([]trading.OrderUpdate, 0),
		observers:       make([]types.Socket, 0),
		bookDepthLevels: 10,
	}
}

func (orders *Orders) Send(message []byte) *types.SocketMessage {
	var in *types.SocketMessage

	if err := sonic.Unmarshal(message, &in); err != nil {
		return nil
	}

	userOrders := make(map[string]trading.OrderUpdate)

	if err := sonic.Unmarshal(in.Data, &userOrders); err != nil {
		return nil
	}

	switch in.Method {
	case "subscribe":
		orders.isActive.Store(true)
	case "unsubscribe":
		orders.isActive.Store(false)
	case "add_order":
		for _, order := range userOrders {
			orders.model = append(orders.model, order)
		}
	case "cancel_order":
		for _, order := range userOrders {
			for i, o := range orders.model {
				if o.OrderID == order.OrderID {
					orders.model = slices.Delete(orders.model, i, 1)
					break
				}
			}
		}
	}

	out := &types.SocketMessage{
		Channel: "orders",
		Success: true,
		Data:    in.Data,
	}

	for _, socket := range orders.observers {
		socket.Send(message)
	}

	return out
}

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		orders.observers = append(orders.observers, socket)
	}
}
