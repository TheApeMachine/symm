package signal

import (
	"context"
	"sync"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type System struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pool     *qpool.Q[any]
	signals  sync.Map
	feedback *market.Feedback
	source   logic.SourceType
}

func NewSystem(
	ctx context.Context,
	pool *qpool.Q[any],
	source logic.SourceType,
	signal func(string) market.Signal,
) *System {
	ctx, cancel := context.WithCancel(ctx)

	return &System{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		source: source,
	}
}

func (system *System) Close() error {
	system.cancel()
	return nil
}
