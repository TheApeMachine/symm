package trader

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/rawbus"
)

type Action struct {
	ctx    context.Context
	cancel context.CancelFunc
	bus    *internal.Bus
}

func NewAction(
	ctx context.Context, bus *internal.Bus,
) *Action {
	ctx, cancel := context.WithCancel(ctx)

	return &Action{
		ctx:    ctx,
		cancel: cancel,
		bus:    bus,
	}
}

func (action *Action) Tick(message *qpool.QValue[any]) error {
	act, err := rawbus.DecodeAction(message)

	if errnie.Error(err) != nil {
		return errnie.Err(
			errnie.Validation,
			"crypto: failed to decode action",
			err,
		)
	}

	if err := errnie.Error(rawbus.Send(
		action.bus, rawbus.TypeOrder, act,
	)); errnie.Error(err) != nil {
		return errnie.Err(
			errnie.IO,
			"crypto: failed to send action",
			err,
		)
	}

	return nil
}
