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
	err     error
	name    string
	status  *StatusTracker
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

func (system *System) Transition(stage Stage) {
	old := system.status.Current()
	system.status.Transition(stage)

	if system.status.Current() == stage && old != stage {
		errnie.Info(fmt.Sprintf("%s: %s -> %s", system.name, old, stage))
	}
}

func (system *System) Status() Stage { return system.status.Current() }

func (system *System) Error(errs ...error) error {
	for _, added := range errs {
		if added == nil {
			continue
		}

		system.err = errors.Join(system.err, errnie.Error(added))

		switch system.status.Current() {
		case DONE, FATAL:
			return system.Close()
		default:
			system.Transition(ERROR)
		}

		categorized, ok := errnie.AsErrnie(added)

		if ok && errnie.IsInternal(categorized) {
			return system.Close()
		}
	}

	return system.err
}

func (system *System) AddCloser(closer io.Closer) {
	system.closers = append(system.closers, closer)
}

func (system *System) System() *System { return system }

func (system *System) Close() error {
	if system.cancel != nil {
		system.cancel()
	}

	closers := system.closers
	system.closers = nil

	for _, closer := range closers {
		if closer == nil {
			continue
		}

		if sysGetter, ok := closer.(interface{ System() *System }); ok && sysGetter.System() == system {
			continue
		}

		system.Error(closer.Close())
	}

	return system.Error()
}
