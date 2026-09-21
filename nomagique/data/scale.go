package data

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ScaleServer multiplies by a factor, and is the simplest thing a graph can wire
into a capability port. It exists so a body port has something concrete to
connect to, and so that anything else implementing Transform is wired exactly
the same way.
*/
type ScaleServer struct {
	*runtime.System
	factor float64
}

func NewScale(ctx context.Context) *ScaleServer {
	server := &ScaleServer{
		System: runtime.NewSystem(ctx, "data.scale"),
		factor: 1,
	}

	server.Transition(runtime.READY)
	return server
}

/*
Apply is the function itself, invoked by whatever holds this capability.
*/
func (server *ScaleServer) Apply(ctx context.Context, call Transform_apply) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.scale.Apply] failed to allocate results",
			err,
		))
	}

	results.SetOut(call.Args().Value() * server.factor)
	return nil
}

/*
Write configures the factor from the graph.
*/
func (server *ScaleServer) Write(ctx context.Context, call Scale_write) error {
	if factor := call.Args().Factor(); factor != 0 {
		server.factor = factor
	}

	return nil
}

/*
Done reports the factor this function applies.
*/
func (server *ScaleServer) Done(ctx context.Context, call Scale_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.scale.Done] failed to allocate results",
			err,
		))
	}

	results.SetFactor(server.factor)
	return nil
}
