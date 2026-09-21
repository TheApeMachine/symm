package controlflow

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ConditionServer evaluates runtime status states or boolean test inputs
and branches them into separate trigger outputs (ready, busy, waiting, error, done).
*/
type ConditionServer struct {
	*runtime.System
	status runtime.Status
	test   bool
}

func NewCondition(ctx context.Context) *ConditionServer {
	server := &ConditionServer{
		System: runtime.NewSystem(ctx, "controlflow.condition"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write receives an incoming status enum and/or boolean test signal.
*/
func (server *ConditionServer) Write(ctx context.Context, call Condition_write) error {
	server.status = call.Args().Status()
	server.test = call.Args().Test()
	return nil
}

/*
Done emits the evaluated condition branch outputs.
*/
func (server *ConditionServer) Done(ctx context.Context, call Condition_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"controlflow.condition: failed to allocate done results",
			err,
		))
	}

	isReady := server.status == runtime.Status_ready || server.test
	isBusy := server.status == runtime.Status_busy
	isWaiting := server.status == runtime.Status_waiting
	isError := server.status == runtime.Status_error || server.status == runtime.Status_fatal
	isDone := server.status == runtime.Status_done

	results.SetReady(isReady)
	results.SetBusy(isBusy)
	results.SetWaiting(isWaiting)
	results.SetError(isError)
	results.SetDone(isDone)

	return nil
}
