package response

import (
	"context"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

/*
Orders simulates Kraken private order methods. Market orders fill immediately at
the desk-provided reference price; resting limits remain tracked until cancelled.
*/
type Orders struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	isActive    atomic.Bool
	model       map[string]trading.OrderUpdate
	executions  map[string]user.Execution
	pendingExec []user.Execution
	observers   []types.Socket
	fillHandler *Balances
}

func NewOrders(ctx context.Context, pool *qpool.Q[any]) *Orders {
	ctx, cancel := context.WithCancel(ctx)

	return &Orders{
		ctx:        ctx,
		cancel:     cancel,
		err:        nil,
		pool:       pool,
		isActive:   atomic.Bool{},
		model:      make(map[string]trading.OrderUpdate),
		executions: make(map[string]user.Execution),
		observers:  make([]types.Socket, 0),
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

		switch typed := frame.Params.(type) {
		case trading.AddParams:
			params = typed
		case *trading.AddParams:
			params = *typed
		default:
			return nil
		}

		if params.ClOrdID == "" {
			return nil
		}

		orders.model[params.ClOrdID] = trading.OrderUpdate{
			OrderID: params.ClOrdID,
		}

		if params.OrderType == trading.Market && params.OrderQty > 0 && params.LimitPrice > 0 {
			orders.fillMarket(params)
		}
	case trading.MethodCancelOrder:
		var params trading.CancelParams

		switch typed := frame.Params.(type) {
		case trading.CancelParams:
			params = typed
		case *trading.CancelParams:
			params = *typed
		default:
			return nil
		}

		for _, orderID := range params.OrderID {
			delete(orders.model, orderID)
		}
	case trading.MethodAmendOrder:
		var params trading.AmendParams

		switch typed := frame.Params.(type) {
		case trading.AmendParams:
			params = typed
		case *trading.AmendParams:
			params = *typed
		default:
			return nil
		}

		if params.OrderID == "" {
			return nil
		}

		orders.model[params.OrderID] = trading.OrderUpdate{
			OrderID: params.OrderID,
		}
	}

	updates := make([]trading.OrderUpdate, 0, len(orders.model))

	for _, update := range orders.model {
		updates = append(updates, update)
	}

	data, err := sonic.Marshal(updates)

	if err != nil {
		return nil
	}

	out := &types.SocketMessage{
		Channel: "orders",
		Success: &[]bool{true}[0],
		Data:    data,
	}

	for _, observer := range orders.observers {
		observer.Send(&qpool.QValue[any]{Value: out})
	}

	return out
}

func (orders *Orders) fillMarket(params trading.AddParams) {
	if orders.fillHandler == nil {
		return
	}

	execution, fillErr := orders.fillHandler.ApplyFill(params, params.LimitPrice)

	if fillErr != nil {
		delete(orders.model, params.ClOrdID)
		return
	}

	orders.executions[execution.ExecID] = execution
	orders.pendingExec = append(orders.pendingExec, execution)
	delete(orders.model, params.ClOrdID)

	balancePayload, err := orders.fillHandler.ModelJSON()

	if err != nil {
		return
	}

	balanceMessage := &types.SocketMessage{
		Channel: "balances",
		Success: &[]bool{true}[0],
		Data:    balancePayload,
	}

	for _, observer := range orders.observers {
		observer.Send(&qpool.QValue[any]{Value: balanceMessage})
	}
}

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		if balances, ok := socket.(*Balances); ok {
			orders.fillHandler = balances
		}

		orders.observers = append(orders.observers, socket)
	}
}

func (orders *Orders) DrainExecutions() []user.Execution {
	if len(orders.pendingExec) == 0 {
		return nil
	}

	rows := append([]user.Execution(nil), orders.pendingExec...)
	orders.pendingExec = orders.pendingExec[:0]

	return rows
}

func (orders *Orders) Wallet() user.Balances {
	if orders.fillHandler == nil {
		return user.Balances{}
	}

	return orders.fillHandler.Wallet()
}
