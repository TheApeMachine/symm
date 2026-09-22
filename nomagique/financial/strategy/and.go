package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AndServer combines actions and emits recommendations when all inputs agree.
*/
type AndServer struct {
	*runtime.System
	result int64
}

func NewAnd(ctx context.Context) *AndServer {
	server := &AndServer{
		System: runtime.NewSystem(ctx, "financial.strategy.and"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *AndServer) Write(ctx context.Context, call And_write) error {
	action1Val := call.Args().Action1()
	action2Val := call.Args().Action2()

	action := int64(indicator.Hold)

	if action1Val == int64(indicator.Sell) && action2Val == int64(indicator.Sell) {
		action = int64(indicator.Sell)
	}

	if action1Val == int64(indicator.Buy) && action2Val == int64(indicator.Buy) {
		action = int64(indicator.Buy)
	}

	server.result = action
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *AndServer) Done(ctx context.Context, call And_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.and.Done] failed to allocate done results",
			err,
		))
	}

	results.SetAction(server.result)
	return nil
}
