package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DenormalizeActionsServer forward-fills the action sequence until a new non-hold action arrives.
*/
type DenormalizeActionsServer struct {
	*runtime.System
	actions chan indicator.Action
	out <-chan indicator.Action
	result int64
}

func NewDenormalizeActions(ctx context.Context) *DenormalizeActionsServer {
	actions := make(chan indicator.Action, 1)

	server := &DenormalizeActionsServer{
		System: runtime.NewSystem(ctx, "financial.strategy.denormalize_actions"),
		actions: actions,
		out: indicator.DenormalizeActionsWithContext(ctx, actions),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *DenormalizeActionsServer) Write(ctx context.Context, call DenormalizeActions_write) error {
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
				"[financial.strategy.denormalize_actions.Write] calculator channel closed",
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
func (server *DenormalizeActionsServer) Done(ctx context.Context, call DenormalizeActions_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.denormalize_actions.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
