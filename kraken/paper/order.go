package paper

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

/*
Orders simulates Kraken private order methods and streams fills on executions.
*/
type Orders struct {
	ctx        context.Context
	socket     *WebSocket
	balances   *Balances
	quotes     *broker.QuoteCache
	catalog    *PairCatalog
	identifier *Identifier
	mu         sync.RWMutex
	open       map[string]*openOrder
}

func NewOrders(
	ctx context.Context,
	socket *WebSocket,
	balances *Balances,
	quotes *broker.QuoteCache,
	catalog *PairCatalog,
	identifier *Identifier,
) *Orders {
	return &Orders{
		ctx:        ctx,
		socket:     socket,
		balances:   balances,
		quotes:     quotes,
		catalog:    catalog,
		identifier: identifier,
		open:       make(map[string]*openOrder),
	}
}

func (orders *Orders) Send(message *qpool.QValue[any]) public.SocketMessage {
	if envelope, ok := message.Value.(public.SocketMessage); ok {
		return envelope
	}

	if execution, ok := message.Value.(user.Execution); ok {
		return orders.executionMessage(execution)
	}

	frame := paramsMap(message.Value)

	if frame == nil {
		return public.SocketMessage{}
	}

	method, _ := frame["method"].(string)

	switch method {
	case trading.MethodAddOrder:
		params, ok := addParamsFromAny(frame["params"])

		if !ok {
			return public.SocketMessage{}
		}

		return orders.addOrder(params)
	case trading.MethodAmendOrder:
		return orders.amendOrder(frame["params"])
	case trading.MethodCancelOrder:
		return orders.cancelOrder(frame["params"])
	case trading.MethodCancelAll:
		return orders.cancelAll()
	case trading.MethodCancelAllOrdersAfter:
		return public.SocketMessage{}
	case trading.MethodBatchAdd:
		return orders.batchAdd(frame["params"])
	case trading.MethodBatchCancel:
		return orders.batchCancel(frame["params"])
	case trading.MethodEditOrder:
		return orders.editOrder(frame["params"])
	}

	return public.SocketMessage{}
}

func (orders *Orders) addOrder(params trading.AddParams) public.SocketMessage {
	if params.Symbol == "" || params.OrderQty <= 0 {
		return public.SocketMessage{}
	}

	clOrdID := params.ClOrdID

	if clOrdID == "" {
		clOrdID = orders.identifier.ClOrdID()
	}

	if !orders.restsOnBook(params) {
		params.ClOrdID = clOrdID

		return orders.fillParams(params)
	}

	quote, ok := orders.quotes.Snapshot(params.Symbol)

	if !ok || broker.WouldCrossPostOnly(quote, params.Side, params.LimitPrice) {
		return rejectedExecution(clOrdID, "post-only order would take liquidity")
	}

	meta := orders.catalog.Meta(params.Symbol)

	order := &openOrder{
		orderID:    orders.identifier.OrderID(),
		clOrdID:    clOrdID,
		symbol:     params.Symbol,
		side:       params.Side,
		orderType:  params.OrderType,
		orderQty:   params.OrderQty,
		limitPrice: params.LimitPrice,
		postOnly:   true,
		queue: broker.NewMakerQueueState(
			quote,
			params.Side,
			params.LimitPrice,
			time.Now().Add(public.SharedNetworkLatency().OneWay()).UnixNano(),
			meta.tickSize,
		),
	}

	orders.storeOrder(order)

	return orders.openExecution(order)
}

func (orders *Orders) amendOrder(params any) public.SocketMessage {
	frame := paramsMap(params)

	if frame == nil {
		return public.SocketMessage{}
	}

	order, ok := orders.resolveOrder(frame)

	if !ok {
		return public.SocketMessage{}
	}

	qty, _ := frame["order_qty"].(float64)
	limitPrice, _ := frame["limit_price"].(float64)

	orders.amendStored(order, qty, limitPrice)

	if qty <= 0 {
		qty = order.orderQty
	}

	if limitPrice <= 0 {
		limitPrice = order.limitPrice
	}

	return orders.executionMessage(user.Execution{
		ExecType:    "amended",
		OrderID:     order.orderID,
		ClOrdID:     order.clOrdID,
		Symbol:      order.symbol,
		Side:        string(order.side),
		OrderType:   string(order.orderType),
		OrderQty:    qty,
		LimitPrice:  limitPrice,
		OrderStatus: "open",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (orders *Orders) cancelOrder(params any) public.SocketMessage {
	frame := paramsMap(params)

	if frame == nil {
		return public.SocketMessage{}
	}

	executions := make([]user.Execution, 0)

	for _, orderID := range orderIDs(frame) {
		order, ok := orders.takeOrder(orderID)

		if !ok {
			continue
		}

		executions = append(executions, orders.cancelExecution(order))
	}

	for _, clOrdID := range stringList(frame["cl_ord_id"]) {
		order, ok := orders.orderByClOrdID(clOrdID)

		if !ok {
			continue
		}

		order, ok = orders.takeOrder(order.orderID)

		if !ok {
			continue
		}

		executions = append(executions, orders.cancelExecution(order))
	}

	return orders.executionMessages(executions)
}

func (orders *Orders) cancelAll() public.SocketMessage {
	ids := orders.openOrderIDs()
	executions := make([]user.Execution, 0, len(ids))

	for _, orderID := range ids {
		order, ok := orders.takeOrder(orderID)

		if !ok {
			continue
		}

		executions = append(executions, orders.cancelExecution(order))
	}

	return orders.executionMessages(executions)
}

func (orders *Orders) batchAdd(params any) public.SocketMessage {
	symbol, items, ok := batchOrders(params)

	if !ok {
		return public.SocketMessage{}
	}

	messages := make([]public.SocketMessage, 0, len(items))

	for _, item := range items {
		item.Symbol = symbol
		messages = append(messages, orders.addOrder(item))
	}

	return orders.publishMessages(messages)
}

func (orders *Orders) batchCancel(params any) public.SocketMessage {
	frame := paramsMap(params)

	if frame == nil {
		return public.SocketMessage{}
	}

	executions := make([]user.Execution, 0)

	for _, orderID := range stringList(frame["orders"]) {
		order, ok := orders.takeOrder(orderID)

		if !ok {
			continue
		}

		executions = append(executions, orders.cancelExecution(order))
	}

	for _, clOrdID := range stringList(frame["cl_ord_id"]) {
		order, ok := orders.orderByClOrdID(clOrdID)

		if !ok {
			continue
		}

		order, ok = orders.takeOrder(order.orderID)

		if !ok {
			continue
		}

		executions = append(executions, orders.cancelExecution(order))
	}

	return orders.executionMessages(executions)
}

func (orders *Orders) editOrder(params any) public.SocketMessage {
	frame := paramsMap(params)

	if frame == nil {
		return public.SocketMessage{}
	}

	orderID, _ := frame["order_id"].(string)
	symbol, _ := frame["symbol"].(string)

	if orderID == "" || symbol == "" {
		return public.SocketMessage{}
	}

	order, ok := orders.takeOrder(orderID)

	if !ok {
		return public.SocketMessage{}
	}

	qty, _ := frame["order_qty"].(float64)
	limitPrice, _ := frame["limit_price"].(float64)

	if qty <= 0 {
		qty = order.orderQty
	}

	if limitPrice <= 0 {
		limitPrice = order.limitPrice
	}

	replacement := &openOrder{
		orderID:    orders.identifier.OrderID(),
		clOrdID:    order.clOrdID,
		symbol:     symbol,
		side:       order.side,
		orderType:  order.orderType,
		orderQty:   qty,
		limitPrice: limitPrice,
		postOnly:   order.postOnly,
	}

	orders.storeOrder(replacement)

	return orders.openExecution(replacement)
}

func (orders *Orders) resolveOrder(frame map[string]any) (*openOrder, bool) {
	if orderID, ok := frame["order_id"].(string); ok && orderID != "" {
		return orders.orderByID(orderID)
	}

	if clOrdID, ok := frame["cl_ord_id"].(string); ok && clOrdID != "" {
		return orders.orderByClOrdID(clOrdID)
	}

	return nil, false
}
