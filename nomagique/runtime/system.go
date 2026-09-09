package runtime

import (
	"context"
	"errors"

	"github.com/theapemachine/errnie"
)

type System struct {
	ctx     context.Context
	cancel  context.CancelFunc
	name    string
	err     error
	status  *Status
	closers []func() error
}

func NewSystem(
	ctx context.Context,
	name string,
	closers ...func() error,
) *System {
	ctx, cancel := context.WithCancel(ctx)

	return &System{
		ctx:     ctx,
		cancel:  cancel,
		name:    name,
		status:  NewStatus(),
		closers: closers,
	}
}

func (system *System) Name() string           { return system.name }
func (system *System) Transition(stage Stage) { system.status.Transition(stage) }
func (system *System) Status() Stage          { return system.status.Current() }
func (system *System) Error() error           { return system.err }
func (system *System) Close() error {
	system.cancel()

	for _, closer := range system.closers {
		system.err = errors.Join(system.err, closer())
	}

	return system.err
}

// Fail retains the stage failure on the runtime owner. Processing must stop
// until the owner is replaced or explicitly recovered.
func (system *System) Fail(err error) {
	if err == nil {
		return
	}
	system.err = errnie.Error(err)
	system.Transition(ERROR)
}
