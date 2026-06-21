package trader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/resonance"
)

/*
Crypto orchestrates measurement collection, playbook walks, and broker fills.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	tree        *dmt.Tree
	pool        *qpool.Q[any]
	uiBroadcast *qpool.BroadcastGroup
	desk        *broker.Desk
	story       *market.Story
	signals     *Signal
	resonance   *resonance.Signal
	wallet      *datura.Artifact
	storyTicks  atomic.Uint64
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	return &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		tree:        tree,
		pool:        pool,
		uiBroadcast: pool.CreateBroadcastGroup("ui"),
		desk:        broker.NewDesk(ctx, pool, tree),
		story:       market.NewStory(ctx, pool),
		signals:     NewSignal(ctx, pool, tree),
		resonance: resonance.NewSignal(
			ctx,
			pool,
			tree,
			viper.GetIntSlice("signals.resonance.arch"),
			viper.GetFloat64("signals.resonance.alpha"),
			viper.GetInt("signals.resonance.batch"),
		),
	}
}

func (crypto *Crypto) Run() error {
	interval := viper.GetDuration("market.story.ui_interval")

	if interval <= 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-ticker.C:
			for _, measurement := range crypto.signals.Measure() {
				measurement.WithDestination("ui")

				if err := crypto.uiBroadcast.Send(measurement); err != nil {
					errnie.Error(errnie.Err(
						errnie.IO,
						"trader: ui publish failed",
						err,
					))
				}
			}
		}
	}
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.desk != nil {
		errnie.Error(crypto.desk.Close())
	}

	if crypto.story != nil {
		errnie.Error(crypto.story.Close())
	}

	if crypto.signals != nil {
		errnie.Error(crypto.signals.Close())
	}

	if crypto.resonance != nil {
		errnie.Error(crypto.resonance.Close())
	}

	return nil
}
