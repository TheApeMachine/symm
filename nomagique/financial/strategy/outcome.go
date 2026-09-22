package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
OutcomeServer simulates cumulative returns from price and action streams.
*/
type OutcomeServer struct {
	*runtime.System
	values chan float64
	actions chan indicator.Action
	out <-chan float64
	result float64
}

func NewOutcome(ctx context.Context) *OutcomeServer {
	values := make(chan float64, 1)
	actions := make(chan indicator.Action, 1)

	server := &OutcomeServer{
		System: runtime.NewSystem(ctx, "financial.strategy.outcome"),
		values: values,
		actions: actions,
		out: indicator.OutcomeWithContext(ctx, values, actions),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *OutcomeServer) Write(ctx context.Context, call Outcome_write) error {
	valueVal := call.Args().Value()
	actionVal := call.Args().Action()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.values <- valueVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.actions <- indicator.Action(actionVal):
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res, ok := <-server.out:
		if !ok {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[financial.strategy.outcome.Write] calculator channel closed",
				nil,
			))
		}

		server.result = res
	}
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *OutcomeServer) Done(ctx context.Context, call Outcome_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.outcome.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
