package response

import (
	"context"
	"errors"
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
	limits          *paperTradingLimits
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
		limits:          newPaperTradingLimits(),
	}

	orders.Observe(observers...)

	return orders
}

func (orders *Orders) SetClock(clock func() time.Time) {
	if orders == nil {
		return
	}
	if clock == nil {
		clock = time.Now
	}
	if orders.limits != nil {
		orders.limits.now = clock
	}
	if orders.fills != nil {
		orders.fills.SetClock(clock)
	}
}

func (orders *Orders) Send(artifact *datura.Artifact) *datura.Artifact {
	if orders == nil || artifact == nil || !artifact.IsValid() {
		return nil
	}

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
		if decisionID := datura.Peek[string](artifact, "params", "decision_id"); decisionID != "" {
			orderPayload["decision_id"] = decisionID
		}
		if actionID := datura.Peek[string](artifact, "params", "action_id"); actionID != "" {
			orderPayload["action_id"] = actionID
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

		if edgeKey := datura.Peek[string](artifact, "params", "edge_key"); edgeKey != "" {
			orderPayload["edge_key"] = edgeKey
		}

		now := orders.now()

		order := datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithRole(
			"orders",
		).WithScope(
			symbol,
		).WithPayload(
			orderPayload.Marshal(),
		)

		resting := orders.paperOrderRests(order)

		if err := orders.limits.Add(symbol, resting, now); err != nil {
			out = rejectedExecution(symbol, orderID, order, err)
			break
		}

		if err := orders.fills.Preflight(order); err != nil {
			out = rejectedExecution(symbol, orderID, order, err)
			break
		}

		if resting {
			payload := datura.Map[any]{
				"order_id":     orderID,
				"cl_ord_id":    datura.Peek[string](order, "cl_ord_id"),
				"symbol":       symbol,
				"side":         datura.Peek[string](order, "side"),
				"order_type":   datura.Peek[string](order, "order_type"),
				"qty":          datura.Peek[float64](order, "order_qty"),
				"order_qty":    datura.Peek[float64](order, "order_qty"),
				"order_status": "open",
				"status":       "open",
				"exec_type":    "new",
				"setup_key":    datura.Peek[string](order, "setup_key"),
				"edge_key":     datura.Peek[string](order, "edge_key"),
				"decision_id":  datura.Peek[string](order, "decision_id"),
				"action_id":    datura.Peek[string](order, "action_id"),
				"created_at":   now.UTC().Format(time.RFC3339Nano),
				"timestamp":    now.UTC().Format(time.RFC3339Nano),
			}
			for _, key := range []string{"limit_price", "trigger_price", "trailing_stop"} {
				if value := datura.Peek[float64](order, key); value > 0 {
					payload[key] = value
				}
			}
			if trailing := datura.Peek[float64](order, "trailing_stop"); trailing > 0 {
				payload["trailing_offset"] = trailing
				if peak := orders.paperOrderInitialPeak(order); peak > 0 {
					payload["peak"] = peak
				}
			}
			orders.model.Store(orderID, payload)
			orders.limits.Open(symbol, orderID, datura.Peek[string](order, "cl_ord_id"), now)
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
			"decision_id":    datura.Peek[string](order, "decision_id"),
			"action_id":      datura.Peek[string](order, "action_id"),
			"fee":            datura.Peek[float64](fill, "fee"),
			"fee_ccy":        datura.Peek[string](fill, "fee_ccy"),
			"liquidity_ind":  datura.Peek[string](fill, "liquidity_ind"),
			"slippage_bps":   datura.Peek[float64](fill, "slippage_bps"),
			"depth_coverage": datura.Peek[float64](fill, "depth_coverage"),
			"timestamp":      orders.now().UTC().Format(time.RFC3339Nano),
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
	case "amend_order", "edit_order":
		identifier := cancelOrderID(artifact)
		symbol := datura.Peek[string](artifact, "params", "symbol")
		if symbol == "" {
			symbol = orders.limits.SymbolForOrder(identifier)
		}

		open, known, err := orders.limits.Amend(method, symbol, identifier, orders.now())
		if err != nil {
			out = rejectedExecution(symbol, identifier, artifact, err)
			break
		}
		if !known {
			return nil
		}

		payload := orders.amendedOpenOrderPayload(open, artifact)
		payload["exec_type"] = "amended"
		orders.model.Store(open.orderID, payload)
		out = openOrderUpdate(open.symbol, payload)
	case "cancel_order":
		identifier := cancelOrderID(artifact)
		symbol := datura.Peek[string](artifact, "params", "symbol")
		if symbol == "" {
			symbol = orders.limits.SymbolForOrder(identifier)
		}

		open, known, err := orders.limits.Cancel(symbol, identifier, orders.now())
		if err != nil {
			out = rejectedExecution(symbol, identifier, artifact, err)
			break
		}
		if !known {
			return nil
		}

		orders.model.Delete(open.orderID)
		payload := datura.Map[any]{
			"order_id":     open.orderID,
			"cl_ord_id":    open.clOrdID,
			"symbol":       open.symbol,
			"order_status": "canceled",
			"status":       "canceled",
			"exec_type":    "canceled",
			"decision_id":  datura.Peek[string](artifact, "params", "decision_id"),
			"action_id":    datura.Peek[string](artifact, "params", "action_id"),
			"timestamp":    orders.now().UTC().Format(time.RFC3339Nano),
		}
		out = openOrderUpdate(open.symbol, payload)
	default:
		return nil
	}

	orders.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(out)
		return true
	})

	return out
}

func (orders *Orders) paperOrderInitialPeak(order *datura.Artifact) float64 {
	if orders == nil || orders.fills == nil || order == nil {
		return 0
	}

	symbol := datura.Peek[string](order, "symbol")
	quote, ok := orders.fills.quoteForSymbol(symbol)
	if !ok {
		return 0
	}
	defer quote.Release()

	side := strings.ToLower(strings.TrimSpace(datura.Peek[string](order, "side")))
	switch side {
	case "sell":
		if bid := datura.Peek[float64](quote, "bid"); bid > 0 {
			return bid
		}
	case "buy":
		if ask := datura.Peek[float64](quote, "ask"); ask > 0 {
			return ask
		}
	}

	return datura.Peek[float64](quote, "last")
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

func (orders *Orders) amendedOpenOrderPayload(
	open paperOpenOrder,
	artifact *datura.Artifact,
) datura.Map[any] {
	payload := datura.Map[any]{
		"order_id":     open.orderID,
		"cl_ord_id":    open.clOrdID,
		"symbol":       open.symbol,
		"order_status": "open",
		"timestamp":    orders.now().UTC().Format(time.RFC3339Nano),
	}

	if existing, ok := orders.model.Load(open.orderID); ok {
		if typed, typedOK := existing.(datura.Map[any]); typedOK {
			for key, value := range typed {
				payload[key] = value
			}
			payload["order_status"] = "open"
			payload["timestamp"] = orders.now().UTC().Format(time.RFC3339Nano)
		}
	}

	for _, key := range []string{"limit_price", "trigger_price", "trailing_stop", "order_qty"} {
		if value := datura.Peek[float64](artifact, "params", key); value > 0 {
			payload[key] = value
		}
	}

	return payload
}

func (orders *Orders) now() time.Time {
	if orders != nil && orders.limits != nil && orders.limits.now != nil {
		return orders.limits.now().UTC()
	}

	return time.Now().UTC()
}

func cancelOrderID(artifact *datura.Artifact) string {
	for _, path := range [][]any{
		{"params", "order_id"},
		{"params", "cl_ord_id"},
		{"order_id"},
		{"cl_ord_id"},
	} {
		if id := datura.Peek[string](artifact, path...); id != "" {
			return id
		}
	}

	return ""
}

func openOrderUpdate(symbol string, payload datura.Map[any]) *datura.Artifact {
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

func rejectedExecution(symbol string, orderID string, order *datura.Artifact, reason error) *datura.Artifact {
	rejectReason := ""
	rejectMessage := ""
	var reject preflightReject
	if reason != nil {
		rejectReason = reason.Error()
		rejectMessage = reason.Error()
		if errors.As(reason, &reject) {
			if reject.code != "" {
				rejectReason = reject.code
			}
		}
	}

	payload := datura.Map[any]{
		"order_id":               orderID,
		"exec_id":                orderID,
		"cl_ord_id":              orderString(order, "cl_ord_id"),
		"decision_id":            orderString(order, "decision_id"),
		"action_id":              orderString(order, "action_id"),
		"symbol":                 symbol,
		"side":                   orderString(order, "side"),
		"order_type":             orderString(order, "order_type"),
		"order_qty":              orderFloat(order, "order_qty"),
		"order_status":           "rejected",
		"exec_type":              "rejected",
		"setup_key":              orderString(order, "setup_key"),
		"reject_reason":          rejectReason,
		"reject_message":         rejectMessage,
		"quote_age":              reject.quoteAge,
		"spread_bps":             reject.spreadBps,
		"projected_slippage_bps": reject.projectedSlippageBps,
		"depth_coverage":         reject.depthCoverage,
		"timestamp":              time.Now().UTC().Format(time.RFC3339Nano),
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

func orderString(order *datura.Artifact, key string) string {
	if value := datura.Peek[string](order, key); value != "" {
		return value
	}

	return datura.Peek[string](order, "params", key)
}

func orderFloat(order *datura.Artifact, key string) float64 {
	if value := datura.Peek[float64](order, key); value > 0 {
		return value
	}

	return datura.Peek[float64](order, "params", key)
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
