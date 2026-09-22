package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormalizeActionsServer filters redundant consecutive actions to maintain a consistent sequence.
*/
type NormalizeActionsServer struct {
	*runtime.System
	actions chan indicator.Action
	out <-chan indicator.Action
	result int64
}

func NewNormalizeActions(ctx context.Context) *NormalizeActionsServer {
	actions := make(chan indicator.Action, 1)

	server := &NormalizeActionsServer{
		System: runtime.NewSystem(ctx, "financial.strategy.normalize_actions"),
		actions: actions,
		out: indicator.NormalizeActionsWithContext(ctx, actions),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *NormalizeActionsServer) Write(ctx context.Context, call NormalizeActions_write) error {
	actionVal := call.Args().Action()

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
				"[financial.strategy.normalize_actions.Write] calculator channel closed",
				nil,
			))
		}

		server.result = int64(res)
	}
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *NormalizeActionsServer) Done(ctx context.Context, call NormalizeActions_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.normalize_actions.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
