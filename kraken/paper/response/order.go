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
	"github.com/theapemachine/symm/kraken/types"
)

type Orders struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	pool            *qpool.Q[any]
	isActive        atomic.Bool
	model           []map[string]any
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
		model:           make([]map[string]any, 0),
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
	case "add_order":
		return orders.handleAddOrder(message)
	case "cancel_order":
		return orders.handleCancelOrder(message)
	}

	return nil
}

func (orders *Orders) handleAddOrder(message []byte) *types.SocketMessage {
	wire, ok := parseAddOrder(message)

	if !ok {
		return nil
	}

	orderID := uuid.NewString()
	update := map[string]any{"order_id": orderID}

	orders.model = append(orders.model, update)

	if orders.fills != nil && orders.fills.Preflight(wire) != nil {
		data, marshalErr := sonic.Marshal(map[string]map[string]any{
			orderID: update,
		})

		if marshalErr != nil {
			return nil
		}

		return &types.SocketMessage{
			Channel: "orders",
			Success: false,
			Data:    data,
		}
	}

	orders.scheduleFill(wire, orderID)

	data, err := sonic.Marshal(map[string]map[string]any{
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
	var frame struct {
		Params json.RawMessage `json:"params"`
	}

	if sonic.Unmarshal(message, &frame) != nil {
		return nil
	}

	var params struct {
		OrderID []string `json:"order_id"`
		ClOrdID []string `json:"cl_ord_id"`
	}

	if sonic.Unmarshal(frame.Params, &params) != nil {
		return nil
	}

	for index, stored := range orders.model {
		orderID, _ := stored["order_id"].(string)

		if !cancelParamsMatch(params.OrderID, params.ClOrdID, orderID, "") {
			continue
		}

		orders.model = slices.Delete(orders.model, index, 1)
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

func cancelParamsMatch(orderIDs, clOrdIDs []string, orderID, clOrdID string) bool {
	for _, value := range orderIDs {
		if value == orderID {
			return true
		}
	}

	for _, value := range clOrdIDs {
		if value == clOrdID {
			return true
		}
	}

	return false
}

func (orders *Orders) scheduleFill(wire map[string]any, orderID string) {
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

		notice, err := orders.fills.Simulate(wire)

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
