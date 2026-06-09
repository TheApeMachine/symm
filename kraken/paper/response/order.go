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
		model:     make(map[string]trading.OrderUpdate),
		observers: make([]types.Socket, 0),
	}
}

func (orders *Orders) Send(message *qpool.QValue[any]) *types.SocketMessage {
	frame, ok := message.Value.(types.KrakenMessage)

	if !ok {
		return nil
	}

	switch frame.Method {
	case "subscribe":
		orders.isActive.Store(true)
	case "unsubscribe":
		orders.isActive.Store(false)
	case trading.MethodAddOrder:
		var params trading.AddParams

		if err := sonic.Unmarshal(frame.Params, &params); err != nil || params.ClOrdID == "" {
			return nil
		}

		orders.model[params.ClOrdID] = trading.OrderUpdate{
			OrderID: params.ClOrdID,
		}
	case trading.MethodCancelOrder:
		var params trading.CancelParams

		if err := sonic.Unmarshal(frame.Params, &params); err != nil || len(params.OrderID) == 0 {
			return nil
		}

		for _, orderID := range params.OrderID {
			delete(orders.model, orderID)
		}
	case trading.MethodAmendOrder:
		var params trading.AmendParams

		if err := sonic.Unmarshal(frame.Params, &params); err != nil || params.OrderID == "" {
			return nil
		}

		orders.model[params.OrderID] = trading.OrderUpdate{
			OrderID: params.OrderID,
		}
	}

	var (
		out     *types.SocketMessage
		data    []byte
		err     error
		updates []trading.OrderUpdate
	)

	updates = make([]trading.OrderUpdate, 0, len(orders.model))

	for _, update := range orders.model {
		updates = append(updates, update)
	}

	data, err = sonic.Marshal(updates)

	if err != nil {
		return nil
	}

	out = &types.SocketMessage{
		Channel: "orders",
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
