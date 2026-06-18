package response

import (
	"context"
	"encoding/json"
	"slices"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/theapemachine/datura/dmt"
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
	balances        *Balances
	executions      *Executions
	fills           *FillSimulator
}

func NewOrders(
	ctx context.Context,
	pool *qpool.Q[any],
) *Orders {
	return NewOrdersWithTree(ctx, pool, nil, nil, nil)
}

func NewOrdersWithTree(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	balances *Balances,
	executions *Executions,
) *Orders {
	ctx, cancel := context.WithCancel(ctx)

	return &Orders{
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		model:           make([]trading.OrderUpdate, 0),
		observers:       make([]types.Socket, 0),
		bookDepthLevels: 10,
		balances:        balances,
		executions:      executions,
		fills:           NewFillSimulator(tree),
	}
}

func (orders *Orders) Send(message []byte) *types.SocketMessage {
	var request struct {
		Method string `json:"method"`
	}

	if err := sonic.Unmarshal(message, &request); err != nil {
		return nil
	}

	switch request.Method {
	case "subscribe":
		orders.isActive.Store(true)
	case "unsubscribe":
		orders.isActive.Store(false)
	case trading.MethodAddOrder:
		return orders.handleAddOrder(message)
	case trading.MethodCancelOrder:
		return orders.handleCancelOrder(message)
	}

	return nil
}

func (orders *Orders) handleAddOrder(message []byte) *types.SocketMessage {
	var params trading.AddParams

	if !unmarshalKrakenParams(message, &params) {
		return nil
	}

	orderID := uuid.NewString()
	update := trading.OrderUpdate{OrderID: orderID}

	orders.model = append(orders.model, update)
	orders.scheduleFill(params, orderID)

	data, err := sonic.Marshal(map[string]trading.OrderUpdate{
		orderID: update,
	})

	if err != nil {
		return nil
	}

	return &types.SocketMessage{
		Channel: "orders",
		Success: true,
		Data:    data,
	}
}

func (orders *Orders) handleCancelOrder(message []byte) *types.SocketMessage {
	var params trading.CancelParams

	if !unmarshalKrakenParams(message, &params) {
		return nil
	}

	for i, stored := range orders.model {
		if !cancelParamsMatch(params, stored.OrderID, "") {
			continue
		}

		orders.model = slices.Delete(orders.model, i, 1)
		break
	}

	data, err := sonic.Marshal(params)

	if err != nil {
		return nil
	}

	return &types.SocketMessage{
		Channel: "orders",
		Success: true,
		Data:    data,
	}
}

func unmarshalKrakenParams(message []byte, params any) bool {
	var frame struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}

	if err := sonic.Unmarshal(message, &frame); err != nil {
		return false
	}

	return sonic.Unmarshal(frame.Params, params) == nil
}

func cancelParamsMatch(params trading.CancelParams, orderID, clOrdID string) bool {
	for _, value := range params.OrderID {
		if value == orderID {
			return true
		}
	}

	for _, value := range params.ClOrdID {
		if value == clOrdID {
			return true
		}
	}

	return false
}

func (orders *Orders) scheduleFill(params trading.AddParams, orderID string) {
	if orders.fills == nil {
		return
	}

	delay := orders.fills.LatencyDelay()

	go func() {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()

			select {
			case <-orders.ctx.Done():
				return
			case <-timer.C:
			}
		}

		notice, err := orders.fills.Simulate(params)

		if err != nil {
			return
		}

		notice.OrderID = orderID
		orders.publishFill(notice)
	}()
}

func (orders *Orders) publishFill(notice FillNotice) {
	if orders.balances != nil {
		orders.balances.ApplyFill(notice)
		orders.balances.PublishUpdate()
	}

	if orders.executions == nil {
		return
	}

	orders.executions.PublishFill(fillExecution(notice))
}

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		orders.observers = append(orders.observers, socket)
	}
}
