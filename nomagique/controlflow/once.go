package controlflow

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
OnceServer implements a single-shot latch gate. When trigger fires,
it allows the held payload to pass through once and latches closed
until an explicit reset signal arrives.
*/
type OnceServer struct {
	*runtime.System
	hasFired   bool
	through    []byte
	shouldEmit bool
}

func NewOnce(ctx context.Context) *OnceServer {
	server := &OnceServer{
		System: runtime.NewSystem(ctx, "controlflow.once"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write updates the through payload, accepts a trigger signal, or resets the latch.
*/
func (server *OnceServer) Write(ctx context.Context, call Once_write) error {
	reset := call.Args().Reset()

	if reset {
		server.hasFired = false
		server.shouldEmit = false
	}

	through, err := call.Args().Through()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "controlflow.once: payload", err))
	}

	if len(through) > 0 {
		server.through = bytes.Clone(through)
	}

	trigger := call.Args().Trigger()

	if trigger && !server.hasFired {
		server.shouldEmit = true
		server.hasFired = true
	}

	return nil
}

/*
Done emits the through payload if triggered for the first time, along with the fired state.
*/
func (server *OnceServer) Done(ctx context.Context, call Once_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"controlflow.once: failed to allocate done results",
			err,
		))
	}

	results.SetIdle()
	results.SetFired(server.hasFired)

	if server.shouldEmit && len(server.through) > 0 {
		server.shouldEmit = false

		if err := results.SetOut(server.through); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"controlflow.once: failed to set out payload",
				err,
			))
		}
	}

	return nil
}
