package response

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

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

func (orders *Orders) Send(message []byte) *types.SocketMessage {
	incoming := datura.Map[any]{}

	if err := sonic.Unmarshal(message, &incoming); err != nil {
		return nil
	}

	artifact := datura.Acquire(
		"kraken:private", datura.APPJSON,
	)

	switch incoming["mesthod"] {
	case "subscribe":
		if incoming["result"].(map[string]any)["channel"] == "execution" {
			// We need to know if the user has subscribed to the executions
			// channel, otherwise we should not be emitting executions.
			orders.execSub = true
			return &types.SocketMessage{}
		}

		orders.isActive.Store(true)
	case "unsubscribe":
		orders.isActive.Store(false)
	case "add_order":
		orders.model.Store(
			incoming["order_id"],
			incoming,
		)
	case "cancel_order":
		orders.model.Delete(incoming["order_id"])
	}

	artifact.WithPayload(datura.Map[any]{
		"order_id":      incoming["order_id"],
		"exec_id":       incoming["order_id"],
		"cl_ord_id":     incoming["cl_ord_id"],
		"symbol":        incoming["symbol"],
		"side":          incoming["side"],
		"order_type":    incoming["order_type"],
		"order_qty":     incoming["order_qty"],
		"order_status":  incoming["order_status"],
		"exec_type":     incoming["exec_type"],
		"last_qty":      incoming["order_qty"],
		"last_price":    incoming["last_price"],
		"avg_price":     incoming["avg_price"],
		"cum_qty":       incoming["order_qty"],
		"fee":           incoming["fee"],
		"fee_ccy":       incoming["fee_ccy"],
		"liquidity_ind": incoming["liquidity_ind"],
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
	}.Marshal())

	orders.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact.Pack())
		return true
	})

	return nil
}

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		orders.observers.Store(uuid.NewString(), socket)
	}
}
