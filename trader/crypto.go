package trader

import (
	"context"
	"sync"
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
	ctx                 context.Context
	cancel              context.CancelFunc
	tree                *dmt.Tree
	pool                *qpool.Q[any]
	uiBroadcast         *qpool.BroadcastGroup
	broadcasts          *sync.Map
	desk                *broker.Desk
	story               *market.Story
	signals             *Signal
	resonance           *resonance.Signal
	storyTicks          atomic.Uint64
	playbookEvaluations atomic.Uint64
	publishedTree       atomic.Bool
	pairs               *sync.Map
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		tree:        tree,
		pool:        pool,
		uiBroadcast: pool.CreateBroadcastGroup("ui"),
		broadcasts:  &sync.Map{},
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
		pairs: &sync.Map{},
	}

	for _, channel := range []string{"kraken:public"} {
		crypto.broadcasts.Store(channel, crypto.pool.CreateBroadcastGroup(channel))
	}

	return crypto
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
			if err := crypto.subscribeToStreams(); err != nil {
				errnie.Error(err)
			}

			artifacts := crypto.signals.Measure()

			for _, artifact := range artifacts {
				crypto.uiBroadcast.Send(artifact.WithDestination("ui"))
			}
		}
	}
}

func (crypto *Crypto) subscribeToStreams() error {
	for instrument := range crypto.tree.Seek([]byte("instrument/snapshot")) {
		symbols := make([]string, 0)

		for _, pair := range datura.Peek[[]map[string]any](instrument, "data", "pairs") {
			if pair["quote"].(string) == viper.GetString("market.quote_currency") {
				if _, ok := crypto.pairs.LoadOrStore(pair["symbol"].(string), pair); !ok {
					symbols = append(symbols, pair["symbol"].(string))
				}
			}
		}

		if len(symbols) > 0 {
			bg, _ := crypto.broadcasts.LoadOrStore(
				"kraken:public", crypto.pool.CreateBroadcastGroup("kraken:public"),
			)

			for _, stream := range []string{"ohlc", "ticker", "book", "trade"} {
				bg.(*qpool.BroadcastGroup).Send(datura.Acquire(
					"trader", datura.APPJSON,
				).WithDestination(
					"kraken:public",
				).WithPayload(
					[]byte(`{"method": "subscribe","params": {"channel": "` + stream + `", "snapshot": true}}`)),
				)
			}
		}

		crypto.tree.Insert([]byte("instrument/snapshot"), nil)
	}

	return nil
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
