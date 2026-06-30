package response

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	tree            *dmt.Tree
	isActive        atomic.Bool
	model           *sync.Map
	pending         *sync.Map
	observers       *sync.Map
	bookDepthLevels int
	fills           *FillSimulator
	execSub         bool
}

func NewOrders(
	ctx context.Context,
	pool *qpool.Q[any],
) *Orders {
	return NewOrdersWithTree(ctx, pool, nil)
}

func NewOrdersWithTree(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	observers ...types.Socket,
) *Orders {
	ctx, cancel := context.WithCancel(ctx)

	orders := &Orders{
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		tree:            tree,
		model:           &sync.Map{},
		pending:         &sync.Map{},
		observers:       &sync.Map{},
		bookDepthLevels: 10,
		fills:           NewFillSimulator(ctx, tree),
	}

	orders.Observe(observers...)

	return orders
}

func (orders *Orders) Send(artifact *datura.Artifact) *datura.Artifact {
	method := datura.Peek[string](artifact, "method")
	var out *datura.Artifact

	switch method {
	case "subscribe":
		channel := datura.Peek[string](artifact, "params", "channel")

		if channel == "execution" || channel == "executions" {
			// We need to know if the user has subscribed to the executions
			// channel, otherwise we should not be emitting executions.
			orders.execSub = true
			return nil
		}

		orders.isActive.Store(true)
		return nil
	case "unsubscribe":
		orders.isActive.Store(false)
		return nil
	case "add_order":
		orderID := uuid.NewString()
		symbol := datura.Peek[string](artifact, "params", "symbol")
		orderPayload := datura.Map[any]{
			"symbol":     symbol,
			"side":       datura.Peek[string](artifact, "params", "side"),
			"order_type": datura.Peek[string](artifact, "params", "order_type"),
			"order_qty":  datura.Peek[float64](artifact, "params", "order_qty"),
			"cl_ord_id":  datura.Peek[string](artifact, "params", "cl_ord_id"),
			"order_id":   orderID,
		}

		if limitPrice := datura.Peek[float64](artifact, "params", "limit_price"); limitPrice > 0 {
			orderPayload["limit_price"] = limitPrice
		}
		if triggerPrice := datura.Peek[float64](artifact, "params", "trigger_price"); triggerPrice > 0 {
			orderPayload["trigger_price"] = triggerPrice
		}
		if trailingStop := datura.Peek[float64](artifact, "params", "trailing_stop"); trailingStop > 0 {
			orderPayload["trailing_stop"] = trailingStop
		}
		if setupKey := datura.Peek[string](artifact, "params", "setup_key"); setupKey != "" {
			orderPayload["setup_key"] = setupKey
		}

		order := datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithRole(
			"orders",
		).WithScope(
			symbol,
		).WithPayload(orderPayload.Marshal())

		if err := orders.fills.Preflight(order); err != nil {
			out = rejectedExecution(symbol, orderID, order, err)
			break
		}

		if orders.paperOrderRests(order) {
			payload := datura.Map[any]{
				"order_id":     orderID,
				"cl_ord_id":    datura.Peek[string](order, "cl_ord_id"),
				"symbol":       symbol,
				"side":         datura.Peek[string](order, "side"),
				"order_type":   datura.Peek[string](order, "order_type"),
				"order_qty":    datura.Peek[float64](order, "order_qty"),
				"order_status": "open",
				"setup_key":    datura.Peek[string](order, "setup_key"),
				"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
			}
			orders.model.Store(orderID, payload)
			out = openOrderUpdate(symbol, payload)
			break
		}

		fill, err := orders.fills.Simulate(order, orderID)
		if err != nil {
			out = rejectedExecution(symbol, orderID, order, err)
			break
		}
		defer fill.Release()

		price := datura.Peek[float64](fill, "last_price")
		if price <= 0 {
			out = rejectedExecution(symbol, orderID, order, errZeroFillPrice(symbol))
			break
		}

		payload := datura.Map[any]{
			"order_id":       orderID,
			"exec_id":        orderID,
			"cl_ord_id":      datura.Peek[string](fill, "cl_ord_id"),
			"symbol":         symbol,
			"side":           datura.Peek[string](fill, "side"),
			"order_type":     datura.Peek[string](fill, "order_type"),
			"order_qty":      datura.Peek[float64](fill, "order_qty"),
			"order_status":   "filled",
			"exec_type":      "trade",
			"last_qty":       datura.Peek[float64](fill, "order_qty"),
			"last_price":     price,
			"avg_price":      datura.Peek[float64](fill, "avg_price"),
			"cum_qty":        datura.Peek[float64](fill, "order_qty"),
			"setup_key":      datura.Peek[string](fill, "setup_key"),
			"fee":            datura.Peek[float64](fill, "fee"),
			"fee_ccy":        datura.Peek[string](fill, "fee_ccy"),
			"liquidity_ind":  datura.Peek[string](fill, "liquidity_ind"),
			"slippage_bps":   datura.Peek[float64](fill, "slippage_bps"),
			"depth_coverage": datura.Peek[float64](fill, "depth_coverage"),
			"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
		}

		orders.model.Store(orderID, payload)
		out = datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithRole(
			"executions",
		).WithScope(
			symbol,
		).WithPayload(datura.Map[any]{
			"channel": "executions",
			"type":    "update",
			"data":    []datura.Map[any]{payload},
		}.Marshal())
	case "cancel_order":
		orders.model.Delete(datura.Peek[string](artifact, "params", "order_id"))
		return nil
	default:
		return nil
	}

	orders.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(out)
		return true
	})

	return out
}

func (orders *Orders) paperOrderRests(order *datura.Artifact) bool {
	if orders == nil || order == nil {
		return false
	}

	orderType := strings.ToLower(strings.TrimSpace(datura.Peek[string](order, "order_type")))
	switch orderType {
	case "market", "settle-position":
		return false
	case "limit":
		return !orders.limitOrderMarketable(order)
	default:
		return true
	}
}

func (orders *Orders) limitOrderMarketable(order *datura.Artifact) bool {
	if orders == nil || orders.fills == nil || order == nil {
		return false
	}

	limit := datura.Peek[float64](order, "limit_price")
	if limit <= 0 {
		return false
	}

	symbol := datura.Peek[string](order, "symbol")
	quote, ok := orders.fills.quoteForSymbol(symbol)
	if !ok {
		return false
	}
	defer quote.Release()

	side := strings.ToLower(strings.TrimSpace(datura.Peek[string](order, "side")))
	switch side {
	case "buy":
		ask := datura.Peek[float64](quote, "ask")
		if ask <= 0 {
			ask = datura.Peek[float64](quote, "last")
		}
		return ask > 0 && limit >= ask
	case "sell":
		bid := datura.Peek[float64](quote, "bid")
		if bid <= 0 {
			bid = datura.Peek[float64](quote, "last")
		}
		return bid > 0 && limit <= bid
	default:
		return false
	}
}

func openOrderUpdate(symbol string, payload datura.Map[any]) *datura.Artifact {
	return datura.Acquire(
		"kraken:private", datura.APPJSON,
	).WithRole(
		"orders",
	).WithScope(
		symbol,
	).WithPayload(datura.Map[any]{
		"channel": "orders",
		"type":    "update",
		"data":    []datura.Map[any]{payload},
	}.Marshal())
}

func rejectedExecution(symbol string, orderID string, order *datura.Artifact, reason error) *datura.Artifact {
	rejectReason := ""
	if reason != nil {
		rejectReason = reason.Error()
	}

	payload := datura.Map[any]{
		"order_id":      orderID,
		"exec_id":       orderID,
		"cl_ord_id":     datura.Peek[string](order, "cl_ord_id"),
		"symbol":        symbol,
		"side":          datura.Peek[string](order, "side"),
		"order_type":    datura.Peek[string](order, "order_type"),
		"order_qty":     datura.Peek[float64](order, "order_qty"),
		"order_status":  "rejected",
		"exec_type":     "rejected",
		"setup_key":     datura.Peek[string](order, "setup_key"),
		"reject_reason": rejectReason,
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
	}

	return datura.Acquire(
		"kraken:private", datura.APPJSON,
	).WithRole(
		"executions",
	).WithScope(
		symbol,
	).WithPayload(datura.Map[any]{
		"channel": "executions",
		"type":    "update",
		"data":    []datura.Map[any]{payload},
	}.Marshal())
}

func errZeroFillPrice(symbol string) error {
	return fillPriceError("paper: zero fill price for " + symbol)
}

type fillPriceError string

func (err fillPriceError) Error() string {
	return string(err)
}

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		orders.observers.Store(uuid.NewString(), socket)
	}
}
