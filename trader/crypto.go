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

			measurements := make([]*datura.Artifact, 0)

			crypto.signals.MeasureEach(func(artifact *datura.Artifact) {
				measurements = append(measurements, artifact)
				errnie.Error(crypto.uiBroadcast.Send(artifact.WithDestination("ui")))
			})

			trace := crypto.publishDecisionTrace(measurements)

			if trace != nil {
				errnie.Error(crypto.uiBroadcast.Send(trace.WithDestination("ui")))
			}
		}
	}
}

func (crypto *Crypto) subscribeToStreams() error {
	quoteCurrency := viper.GetString("market.quote_currency")
	limit := viper.GetInt("market.max_scan_symbols")

	for instrument := range crypto.tree.Seek([]byte("instrument/snapshot")) {
		symbols := make([]string, 0)

		for index := range datura.Peek[[]any](instrument, "data", "pairs") {
			if limit > 0 && len(symbols) >= limit {
				break
			}

			if datura.Peek[string](instrument, "data", "pairs", index, "quote") != quoteCurrency {
				continue
			}

			symbol := datura.Peek[string](instrument, "data", "pairs", index, "symbol")

			if symbol == "" {
				continue
			}

			if _, ok := crypto.pairs.LoadOrStore(symbol, symbol); !ok {
				symbols = append(symbols, symbol)
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
					datura.Map[any]{
						"method": "subscribe",
						"params": datura.Map[any]{
							"channel":  stream,
							"snapshot": true,
							"symbol":   symbols,
						},
					}.Marshal()),
				)
			}
		}
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
