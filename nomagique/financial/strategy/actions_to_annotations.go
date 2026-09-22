package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ActionsToAnnotationsServer maps actions to human-readable string annotations.
*/
type ActionsToAnnotationsServer struct {
	*runtime.System
	actions chan indicator.Action
	out <-chan string
	result string
}

func NewActionsToAnnotations(ctx context.Context) *ActionsToAnnotationsServer {
	actions := make(chan indicator.Action, 1)

	server := &ActionsToAnnotationsServer{
		System: runtime.NewSystem(ctx, "financial.strategy.actions_to_annotations"),
		actions: actions,
		out: indicator.ActionsToAnnotationsWithContext(ctx, actions),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *ActionsToAnnotationsServer) Write(ctx context.Context, call ActionsToAnnotations_write) error {
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
				"[financial.strategy.actions_to_annotations.Write] calculator channel closed",
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
func (server *ActionsToAnnotationsServer) Done(ctx context.Context, call ActionsToAnnotations_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.actions_to_annotations.Done] failed to allocate done results",
			err,
		))
	}

	if err := results.SetAnnotation(server.result); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.actions_to_annotations.Done] failed to set annotation",
			err,
		))
	}
	return nil
}
