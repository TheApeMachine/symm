package optimizer

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

type Trader struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
}

func NewTrader(ctx context.Context, pool *qpool.Q) (*Trader, error) {
	ctx, cancel := context.WithCancel(ctx)

	trader := &Trader{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
	}

	for _, channel := range []string{"actions"} {
		trader.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		trader.subscribers[channel] = trader.broadcasts[channel].Subscribe("actions", 128)
	}

	return trader, errnie.Error(errnie.Require((map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"pool":   pool,
	})))
}

func (trader *Trader) Tick() error {
	for row := range trader.subscribers["actions"].Incoming {
		if row == nil {
			continue
		}
	}

	return trader.ctx.Err()
}
