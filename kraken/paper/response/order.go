package response

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/theapemachine/datura"
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
		fills:           NewFillSimulator(ctx, tree),
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
	order, ok := orders.orderFromMessage(message)

	if !ok {
		return nil
	}

	defer order.Release()

	orderID := uuid.NewString()
	update := map[string]any{"order_id": orderID}

	orders.model = append(orders.model, update)

	if orders.fills != nil && orders.fills.Preflight(order) != nil {
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

	orders.scheduleFill(order, orderID)

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
	if slices.Contains(orderIDs, orderID) {
		return true
	}

	return slices.Contains(clOrdIDs, clOrdID)
}

func (orders *Orders) orderFromMessage(message []byte) (*datura.Artifact, bool) {
	envelope := datura.Acquire("kraken", datura.Artifact_Type_json).WithPayload(message)

	if datura.Peek[string](envelope, "method") != "add_order" {
		envelope.Release()

		return nil, false
	}

	order := datura.Acquire("paper", datura.Artifact_Type_json)
	order.WithRole("order")
	order.Poke(datura.Peek[string](envelope, "params", "symbol"), "symbol")
	order.Poke(datura.Peek[string](envelope, "params", "side"), "side")
	order.Poke(orders.orderQty(envelope), "order_qty")
	order.Poke(datura.Peek[string](envelope, "params", "cl_ord_id"), "cl_ord_id")
	order.Poke(datura.Peek[string](envelope, "params", "order_type"), "order_type")
	order.Poke(datura.Peek[string](envelope, "params", "action_type"), "action_type")

	envelope.Release()

	return order, true
}

func (orders *Orders) orderQty(envelope *datura.Artifact) float64 {
	if envelope == nil {
		return 0
	}

	if value := datura.Peek[float64](envelope, "params", "order_qty"); value != 0 {
		return value
	}

	if value := datura.Peek[int64](envelope, "params", "order_qty"); value != 0 {
		return float64(value)
	}

	raw := datura.Peek[string](envelope, "params", "order_qty")

	if raw == "" {
		return 0
	}

	parsed, err := strconv.ParseFloat(raw, 64)

	if err != nil {
		return 0
	}

	return parsed
}

func (orders *Orders) orderQtyFromArtifact(order *datura.Artifact) float64 {
	if order == nil {
		return 0
	}

	if value := datura.Peek[float64](order, "order_qty"); value != 0 {
		return value
	}

	if value := datura.Peek[int64](order, "order_qty"); value != 0 {
		return float64(value)
	}

	raw := datura.Peek[string](order, "order_qty")

	if raw == "" {
		return 0
	}

	parsed, err := strconv.ParseFloat(raw, 64)

	if err != nil {
		return 0
	}

	return parsed
}

func (orders *Orders) scheduleFill(order *datura.Artifact, orderID string) {
	if orders.fills == nil || order == nil {
		return
	}

	symbol := datura.Peek[string](order, "symbol")

	if symbol == "" {
		return
	}

	side := datura.Peek[string](order, "side")
	orderQty := orders.orderQtyFromArtifact(order)
	clOrdID := datura.Peek[string](order, "cl_ord_id")
	orderType := datura.Peek[string](order, "order_type")
	actionType := datura.Peek[string](order, "action_type")

	go func() {
		if orders.fills.latency != nil {
			orders.fills.latency.Wait()
		}

		request := datura.Acquire("paper", datura.Artifact_Type_json)
		request.WithRole("order")
		request.Poke(symbol, "symbol")
		request.Poke(side, "side")
		request.Poke(orderQty, "order_qty")
		request.Poke(clOrdID, "cl_ord_id")
		request.Poke(orderType, "order_type")
		request.Poke(actionType, "action_type")

		fill, err := orders.fills.Simulate(request, orderID)

		request.Release()

		if err != nil {
			return
		}

		fill.Poke(orderID, "order_id")
		fill.Poke(orderID, "exec_id")
		orders.publishFill(fill)
	}()
}

func (orders *Orders) publishFill(fill *datura.Artifact) {
	if fill == nil {
		return
	}

	defer fill.Release()

	if orders.balances != nil {
		orders.balances.ApplyFill(fill)
		orders.balances.PublishUpdate()
	}

	if orders.executions == nil {
		return
	}

	orders.executions.PublishFill(fill)
}

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		orders.observers = append(orders.observers, socket)
	}
}
