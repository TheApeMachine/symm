package response

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/frame"
	"github.com/theapemachine/symm/kraken/types"
)

/*
Executions simulates the Kraken executions channel and publishes the same raw
frames and derived envelopes as the live private websocket.
*/
type Executions struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	pool      *qpool.Q[any]
	tree      *dmt.Tree
	isActive  atomic.Bool
	model     []map[string]any
	observers []types.Socket
}

func NewExecutions(ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree) *Executions {
	ctx, cancel := context.WithCancel(ctx)

	return &Executions{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		tree:      tree,
		model:     make([]map[string]any, 0),
		observers: make([]types.Socket, 0),
	}
}

func (executions *Executions) Send(message []byte) *types.SocketMessage {
	var in *types.SocketMessage

	if err := sonic.Unmarshal(message, &in); err != nil {
		return nil
	}

	incoming := make(map[string]map[string]any)

	if err := sonic.Unmarshal(in.Data, &incoming); err != nil {
		return nil
	}

	switch in.Method {
	case "subscribe":
		executions.isActive.Store(true)
	case "unsubscribe":
		executions.isActive.Store(false)
	case "add_order":
		for _, execution := range incoming {
			executions.model = append(executions.model, execution)
		}
	case "cancel_order":
		for _, execution := range incoming {
			orderID, _ := execution["order_id"].(string)

			for index, stored := range executions.model {
				storedID, _ := stored["order_id"].(string)

				if storedID != orderID {
					continue
				}

				executions.model = slices.Delete(executions.model, index, 1)
				break
			}
		}
	}

	out := &types.SocketMessage{
		Channel: "executions",
		Success: true,
		Data:    in.Data,
	}

	for _, socket := range executions.observers {
		socket.Send(in.Data)
	}

	return out
}

func (executions *Executions) PublishFill(fill *datura.Artifact) {
	if fill == nil {
		return
	}

	execID := datura.Peek[string](fill, "exec_id")

	if execID == "" {
		execID = datura.Peek[string](fill, "order_id")
	}

	execution := map[string]any{
		"order_id":      datura.Peek[string](fill, "order_id"),
		"cl_ord_id":     datura.Peek[string](fill, "cl_ord_id"),
		"symbol":        datura.Peek[string](fill, "symbol"),
		"side":          datura.Peek[string](fill, "side"),
		"order_type":    datura.Peek[string](fill, "order_type"),
		"order_qty":     datura.Peek[float64](fill, "order_qty"),
		"order_status":  datura.Peek[string](fill, "order_status"),
		"exec_type":     datura.Peek[string](fill, "exec_type"),
		"exec_id":       execID,
		"last_qty":      datura.Peek[float64](fill, "order_qty"),
		"last_price":    datura.Peek[float64](fill, "last_price"),
		"avg_price":     datura.Peek[float64](fill, "avg_price"),
		"cum_qty":       datura.Peek[float64](fill, "order_qty"),
		"fee":           datura.Peek[float64](fill, "fee"),
		"fee_ccy":       datura.Peek[string](fill, "fee_ccy"),
		"liquidity_ind": datura.Peek[string](fill, "liquidity_ind"),
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
	}

	executions.model = append(executions.model, execution)

	data, err := sonic.Marshal(map[string]map[string]any{
		execID: execution,
	})

	if err != nil {
		return
	}

	for _, socket := range executions.observers {
		socket.Send(data)
	}

	if !executions.isActive.Load() || executions.pool == nil {
		return
	}

	message := &types.SocketMessage{
		Channel: "executions",
		Type:    "update",
		Success: true,
		Data:    data,
	}

	ui := executions.pool.CreateBroadcastGroup("ui")

	errnie.Error(frame.Publish(executions.tree, ui, nil, message))
}

func (executions *Executions) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		executions.observers = append(executions.observers, socket)
	}
}
