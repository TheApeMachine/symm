package response

import (
	"context"
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

		order := datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithRole(
			"orders",
		).WithScope(
			symbol,
		).WithPayload(datura.Map[any]{
			"symbol":     symbol,
			"side":       datura.Peek[string](artifact, "params", "side"),
			"order_type": datura.Peek[string](artifact, "params", "order_type"),
			"order_qty":  datura.Peek[float64](artifact, "params", "order_qty"),
			"cl_ord_id":  datura.Peek[string](artifact, "params", "cl_ord_id"),
			"order_id":   orderID,
		}.Marshal())

		payload := datura.Map[any]{
			"order_id":      orderID,
			"exec_id":       orderID,
			"cl_ord_id":     datura.Peek[string](order, "cl_ord_id"),
			"symbol":        symbol,
			"side":          datura.Peek[string](order, "side"),
			"order_type":    datura.Peek[string](order, "order_type"),
			"order_qty":     datura.Peek[float64](order, "order_qty"),
			"order_status":  "filled",
			"exec_type":     "trade",
			"last_qty":      datura.Peek[float64](order, "order_qty"),
			"last_price":    datura.Peek[float64](artifact, "params", "limit_price"),
			"avg_price":     datura.Peek[float64](artifact, "params", "limit_price"),
			"cum_qty":       datura.Peek[float64](order, "order_qty"),
			"fee":           0.0,
			"fee_ccy":       "",
			"liquidity_ind": "",
			"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
		}

		if fill, err := orders.fills.Simulate(order, orderID); err == nil {
			for _, key := range []string{
				"last_price", "avg_price", "fee", "fee_ccy", "liquidity_ind",
			} {
				payload[key] = datura.Peek[any](fill, key)
			}

			fill.Release()
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

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		orders.observers.Store(uuid.NewString(), socket)
	}
}
