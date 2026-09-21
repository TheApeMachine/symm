package controlflow

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
LoopServer evaluates iterations of a stream loop. While active, it emits
the payload, tracks the current iteration index, and signals completion
when an optional limit is reached.
*/
type LoopServer struct {
	*runtime.System
	data   []byte
	active bool
	limit  int64
	index  int64
	done   bool
}

func NewLoop(ctx context.Context) *LoopServer {
	return &LoopServer{
		System: runtime.NewSystem(ctx, "controlflow.loop"),
		active: true,
	}
}

/*
Write accepts the data payload to circulate, an active gate, and an optional limit.
*/
func (server *LoopServer) Write(ctx context.Context, call Loop_write) error {
	payload, err := call.Args().Data()
	if err == nil && len(payload) > 0 {
		server.data = bytes.Clone(payload)
	}

	server.limit = call.Args().Limit()
	server.active = call.Args().Active()

	if !server.active {
		server.Transition(runtime.WAITING)
		return nil
	}

	if server.limit > 0 && server.index >= server.limit {
		server.done = true
		server.Transition(runtime.DONE)
		return nil
	}

	server.done = false
	server.Transition(runtime.READY)
	server.index++
	return nil
}

/*
Done returns the circulated payload, current iteration index, and completion status.
*/
func (server *LoopServer) Done(ctx context.Context, call Loop_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"loop: failed to allocate done results",
			err,
		))
	}

	results.SetIndex(server.index)
	results.SetDone(server.done)

	if len(server.data) > 0 && !server.done {
		if err := results.SetOut(server.data); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"loop: failed to set out result",
				err,
			))
		}
	}

	return nil
}
