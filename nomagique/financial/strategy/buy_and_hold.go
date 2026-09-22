package strategy

import (
	"context"
	"github.com/cinar/indicator/v2/asset"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BuyAndHoldServer calculates the baseline Buy and Hold strategy recommendations.
*/
type BuyAndHoldServer struct {
	*runtime.System
	snapshots chan *asset.Snapshot
	out <-chan indicator.Action
	result int64
}

func NewBuyAndHold(ctx context.Context) *BuyAndHoldServer {
	snapshots := make(chan *asset.Snapshot, 1)
	calculator := indicator.NewBuyAndHoldStrategy()

	server := &BuyAndHoldServer{
		System: runtime.NewSystem(ctx, "financial.strategy.buy_and_hold"),
		snapshots: snapshots,
		out: calculator.ComputeWithContext(ctx, snapshots),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *BuyAndHoldServer) Write(ctx context.Context, call BuyAndHold_write) error {
	openVal := call.Args().Open()
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	closeVal := call.Args().Close()
	volumeVal := call.Args().Volume()

	snapshot := &asset.Snapshot{
		Open: openVal,
		High: highVal,
		Low: lowVal,
		Close: closeVal,
		Volume: volumeVal,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.snapshots <- snapshot:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case action, ok := <-server.out:
		if !ok {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[financial.strategy.buy_and_hold.Write] calculator channel closed",
				nil,
			))
		}

		server.result = int64(action)
	}
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *BuyAndHoldServer) Done(ctx context.Context, call BuyAndHold_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.buy_and_hold.Done] failed to allocate done results",
			err,
		))
	}

	results.SetAction(server.result)
	return nil
}
