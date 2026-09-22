package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MajorityServer emits recommendations aligned with the majority vote among inputs.
*/
type MajorityServer struct {
	*runtime.System
	result int64
}

func NewMajority(ctx context.Context) *MajorityServer {
	server := &MajorityServer{
		System: runtime.NewSystem(ctx, "financial.strategy.majority"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MajorityServer) Write(ctx context.Context, call Majority_write) error {
	action1Val := call.Args().Action1()
	action2Val := call.Args().Action2()
	action3Val := call.Args().Action3()

	var buyCount, sellCount, holdCount int
	inputs := []int64{action1Val, action2Val, action3Val}

	for _, item := range inputs {
		if item == int64(indicator.Buy) {
			buyCount++
		}

		if item == int64(indicator.Sell) {
			sellCount++
		}

		if item == int64(indicator.Hold) {
			holdCount++
		}
	}

	action := int64(indicator.Hold)

	if sellCount > buyCount && sellCount > holdCount {
		action = int64(indicator.Sell)
	}

	if buyCount > sellCount && buyCount > holdCount {
		action = int64(indicator.Buy)
	}

	server.result = action
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *MajorityServer) Done(ctx context.Context, call Majority_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.majority.Done] failed to allocate done results",
			err,
		))
	}

	results.SetAction(server.result)
	return nil
}
