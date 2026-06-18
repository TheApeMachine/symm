package response

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
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
	model     []user.Execution
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
		model:     make([]user.Execution, 0),
		observers: make([]types.Socket, 0),
	}
}

func (executions *Executions) Send(message []byte) *types.SocketMessage {

	var in *types.SocketMessage

	if err := sonic.Unmarshal(message, &in); err != nil {
		return nil
	}

	userExecutions := make(map[string]user.Execution)

	if err := sonic.Unmarshal(in.Data, &userExecutions); err != nil {
		return nil
	}

	switch in.Method {
	case "subscribe":
		executions.isActive.Store(true)
	case "unsubscribe":
		executions.isActive.Store(false)
	case "add_order":
		for _, execution := range userExecutions {
			executions.model = append(executions.model, execution)
		}
	case "cancel_order":
		for _, execution := range userExecutions {
			for i, e := range executions.model {
				if e.OrderID == execution.OrderID {
					executions.model = slices.Delete(executions.model, i, 1)
					break
				}
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

func (executions *Executions) PublishFill(execution user.Execution) {
	executions.model = append(executions.model, execution)

	data, err := sonic.Marshal(map[string]user.Execution{
		execution.ExecID: execution,
	})

	if err != nil {
		return
	}

	for _, socket := range executions.observers {
		socket.Send(data)
	}
}

func (executions *Executions) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		executions.observers = append(executions.observers, socket)
	}
}
