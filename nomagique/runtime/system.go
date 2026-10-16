package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/theapemachine/errnie"
)

type RuntimeSystem interface {
	Name() string
	Context() context.Context
	Status() Stage
	Transition(Stage)
	Error(...error) error
	AddCloser(io.Closer)
	Close() error
}

type System struct {
	ctx     context.Context
	cancel  context.CancelFunc
	name    string
	err     error
	status  *Status
	closers []io.Closer
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

func (system *System) Transition(stage Stage) {
	old := system.status.Current()
	system.status.Transition(stage)

	if system.status.Current() == stage && old != stage {
		errnie.Info(fmt.Sprintf("%s: %s -> %s", system.name, old, stage))
	}
}

func (system *System) Status() Stage { return system.status.Current() }

/*
ShuttingDown reports whether err is only the cancellation of ctx during an
orderly shutdown: ctx is done and err is context.Canceled. Such an error is
the expected end of in-flight work, not a failure; anything else is.
*/
func ShuttingDown(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, context.Canceled)
}

func (system *System) Error(errs ...error) error {
	var added bool

	for _, err := range errs {
		if err == nil || ShuttingDown(system.ctx, err) {
			continue
		}

		added = true

		if system.err == nil {
			system.err = err
			continue
		}

		system.err = errors.Join(system.err, err)
	}

	if added && system.err != nil {
		if system.status.Current() != FATAL {
			system.Transition(ERROR)
		}

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

func (system *System) System() *System { return system }

func (system *System) Close() error {
	if system.cancel == nil {
		return system.err
	}

	system.cancel()

	closers := system.closers
	system.closers = nil

	for _, closer := range closers {
		if closer == nil {
			continue
		}

		if sysGetter, ok := closer.(interface{ System() *System }); ok && sysGetter.System() == system {
			continue
		}

		if err := closer.Close(); err != nil {
			errnie.Error(errnie.Err(errnie.IO, fmt.Sprintf("%s: closing %T failed", system.name, closer), err))
			system.err = errors.Join(system.err, err)
		}
	}

	return system.err
}
