package broker

import (
	"context"
	"errors"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	symmmarket "github.com/theapemachine/symm/market"
)

type Desk struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	broadcasts  *sync.Map
	subscribers *sync.Map
}

func NewDesk(
	ctx context.Context,
	pool *qpool.Q[any],
	touchRegistry *symmmarket.TouchRegistry,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  &sync.Map{},
		subscribers: &sync.Map{},
	}

	for _, channel := range []string{"desk", "ui"} {
		desk.broadcasts.Store(
			channel, pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{"orders"} {
		desk.subscribers.Store(
			channel, pool.Subscribe(channel, desk.onMessage),
		)
	}

	return desk
}

func (desk *Desk) onMessage(artifact *datura.Artifact) error {
	origin := errnie.Does(func() (string, error) {
		return artifact.Origin()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: failed to get origin",
			err,
		))
	}).Value()

	switch origin {
	case "trader":
	default:
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: unknown origin",
			errors.New(origin),
		))
	}

	return nil
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
