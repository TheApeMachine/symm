package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SplitServer combines buy and sell strategy action streams.
*/
type SplitServer struct {
	*runtime.System
	result int64
}

func NewSplit(ctx context.Context) *SplitServer {
	server := &SplitServer{
		System: runtime.NewSystem(ctx, "financial.strategy.split"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SplitServer) Write(ctx context.Context, call Split_write) error {
	buyActionVal := call.Args().BuyAction()
	sellActionVal := call.Args().SellAction()

	action := int64(indicator.Hold)

	if buyActionVal == int64(indicator.Buy) && sellActionVal != int64(indicator.Sell) {
		action = int64(indicator.Buy)
	}

	if sellActionVal == int64(indicator.Sell) && buyActionVal != int64(indicator.Buy) {
		action = int64(indicator.Sell)
	}

	server.result = action
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *SplitServer) Done(ctx context.Context, call Split_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.split.Done] failed to allocate done results",
			err,
		))
	}

	results.SetAction(server.result)
	return nil
}
