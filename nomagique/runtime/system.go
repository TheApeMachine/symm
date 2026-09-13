package runtime

import (
	"context"
	"errors"
	"io"

	"github.com/theapemachine/errnie"
)

type System struct {
	ctx     context.Context
	cancel  context.CancelFunc
	name    string
	err     error
	status  *Status
	closers []io.Closer
}

type Closer func() error

func (closer Closer) Close() error {
	if closer == nil {
		return nil
	}

	return closer()
}

func NewSystem(
	ctx context.Context,
	name string,
	closers ...io.Closer,
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

func (system *System) Name() string             { return system.name }
func (system *System) Context() context.Context { return system.ctx }
func (system *System) Transition(stage Stage)   { system.status.Transition(stage) }
func (system *System) Status() Stage            { return system.status.Current() }

func (system *System) Error(errs ...error) error {
	for _, err := range errs {
		system.err = errors.Join(system.err, err)
	}

	if system.err != nil {
		system.Transition(ERROR)
		errnie.Error(system.err)

		errnieErr, ok := errnie.AsErrnie(system.err)

		if ok && errnie.IsInternal(errnieErr) {
			system.Close()
		}
	}

	return system.err
}

func (system *System) AddCloser(closer io.Closer) {
	system.closers = append(system.closers, closer)
}

func (system *System) Close() error {
	system.cancel()

	for _, closer := range system.closers {
		if closer == nil {
			continue
		}

		system.err = errors.Join(system.err, closer.Close())
	}

	return system.err
}
