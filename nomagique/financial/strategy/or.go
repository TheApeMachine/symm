package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
OrServer emits recommendations when at least one input recommends without conflict.
*/
type OrServer struct {
	*runtime.System
	result int64
}

func NewOr(ctx context.Context) *OrServer {
	server := &OrServer{
		System: runtime.NewSystem(ctx, "financial.strategy.or"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *OrServer) Write(ctx context.Context, call Or_write) error {
	action1Val := call.Args().Action1()
	action2Val := call.Args().Action2()

	action := int64(indicator.Hold)

	if (action1Val == int64(indicator.Sell) || action2Val == int64(indicator.Sell)) && action1Val != int64(indicator.Buy) && action2Val != int64(indicator.Buy) {
		action = int64(indicator.Sell)
	}

	if (action1Val == int64(indicator.Buy) || action2Val == int64(indicator.Buy)) && action1Val != int64(indicator.Sell) && action2Val != int64(indicator.Sell) {
		action = int64(indicator.Buy)
	}

	server.result = action
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *OrServer) Done(ctx context.Context, call Or_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.or.Done] failed to allocate done results",
			err,
		))
	}

	results.SetAction(server.result)
	return nil
}
