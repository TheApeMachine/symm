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

const (
	noticeFill = "paper:fill"
	noticeArm  = "paper:arm"
)

/*
FillNotice is an internal observer payload from Orders to Executions.
*/
type FillNotice struct {
	Params       trading.AddParams
	OrderID      string
	Price        float64
	Fee          float64
	Reason       string
	LiquidityInd string
	Maker        bool
	Partial      bool
}

/*
ArmNotice is an internal observer payload when a protective order rests.
*/
type ArmNotice struct {
	Params  trading.AddParams
	OrderID string
}

/*
Executions simulates the Kraken executions channel and publishes the same raw
frames and derived envelopes as the live private websocket.
*/
type Executions struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	pool      *qpool.Q[any]
	isActive  atomic.Bool
	model     map[string]user.Execution
	observers []types.Socket
}

func NewExecutions(ctx context.Context, pool *qpool.Q[any]) *Executions {
	ctx, cancel := context.WithCancel(ctx)

	return &Executions{
		ctx:       ctx,
		cancel:    cancel,
		err:       nil,
		pool:      pool,
		isActive:  atomic.Bool{},
		model:     make(map[string]user.Execution),
		observers: make([]types.Socket, 0),
	}
}

func (executions *Executions) Send(message *qpool.QValue[any]) *types.SocketMessage {
	var (
		out   *types.SocketMessage
		inMsg map[string]any
		ok    bool
	)

	if inMsg, ok = message.Value.(map[string]any); !ok {
		return nil
	}

	switch inMsg["method"].(string) {
	case "subscribe":
		executions.isActive.Store(true)
	case "unsubscribe":
		executions.isActive.Store(false)
		out = &types.SocketMessage{
			Method:  "unsubscribe",
			Success: &[]bool{true}[0],
		}
	case "add_order":
		for _, execution := range executions.model {
			if execution.OrderID == inMsg["order_id"].(string) {
				execution.OrderID = inMsg["order_id"].(string)
				break
			}
		}
	case "cancel_order":
		for _, execution := range executions.model {
			if execution.OrderID == inMsg["order_id"].(string) {
				execution.OrderID = inMsg["order_id"].(string)
				break
			}
		}
	}

	data, err := sonic.Marshal(executions.model)

	if err != nil {
		return nil
	}

	out = &types.SocketMessage{
		Channel: "executions",
		Success: &[]bool{true}[0],
		Data:    data,
	}

	for _, observer := range executions.observers {
		observer.Send(&qpool.QValue[any]{Value: out})
	}

	return out
}

func (executions *Executions) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		executions.observers = append(executions.observers, socket)
	}
}
