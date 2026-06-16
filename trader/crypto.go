package trader

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/prediction"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
)

/*
Crypto is a trader that is responsible for orchestrating
the trading of crypto assets. It should collect the data
it needs to make informed decisions regarding the opening,
closing, and reporting of positions.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	broadcasts  *sync.Map
	subscribers *sync.Map
	instrument  *Instrument
	book        *Book
	ticker      *Ticker
	trade       *Trade
	ohlc        *OHLC
	action      *Action
	execution   *Execution
	balances    *Balances
	signals     *sync.Map
	story       *market.Story
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any],
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  &sync.Map{},
		subscribers: &sync.Map{},
		instrument:  NewInstrument(ctx),
		book:        NewBook(ctx),
		ticker:      NewTicker(ctx),
		trade:       NewTrade(ctx),
		ohlc:        NewOHLC(ctx),
		action:      NewAction(ctx),
		execution:   NewExecution(ctx),
		balances:    NewBalances(ctx),
		signals:     &sync.Map{},
		story:       market.NewStory(ctx, pool),
	}

	for _, channel := range []string{
		"desk", "ui",
	} {
		crypto.broadcasts.Store(
			channel, pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{
		"ticker", "book", "trade", "action", "execution", "balances",
	} {
		crypto.subscribers.Store(
			channel, pool.Subscribe(channel, crypto.onMessage),
		)
	}

	return crypto
}

func (crypto *Crypto) Run() error {
	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		default:
		}

		for _, action := range crypto.story.Actions() {
			// TODO: Make trading decisions based on the proposed actions.
			// The story only proposes actions, based on the decision tree.
			// The trader still evaluates what are the best actions to take,
			// based on factors like risk, reward, market conditions, its
			// open positions, etc.
		}
	}

	return nil
}

func (crypto *Crypto) onMessage(artifact *datura.Artifact) error {
	origin := errnie.Does(func() (string, error) {
		return artifact.Origin()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: failed to get origin",
			err,
		))
	}).Value()

	switch origin {
	case "ticker":
		updates := datura.As[krakenmarket.TickerUpdates](artifact)
		crypto.ticker.Update(updates)
		crypto.updateSignals(
			artifact,
			"causal",
			"correlation",
			"depthflow",
			"exhaust",
			"fluid",
			"leadlag",
			"liquidity",
			"manifold",
			"sentiment",
			"toxicity",
		)
	case "book":
		updates := datura.As[krakenmarket.BookUpdates](artifact)
		crypto.book.Update(updates)
		crypto.updateSignals(
			artifact,
			"causal",
			"depthflow",
			"exhaust",
			"fluid",
			"leadlag",
			"liquidity",
			"manifold",
			"pumpdump",
			"sentiment",
			"toxicity",
		)
	case "trade":
		updates := datura.As[krakenmarket.TradeUpdates](artifact)
		crypto.trade.Update(updates)
		crypto.updateSignals(
			artifact,
			"causal",
			"correlation",
			"cvd",
			"depthflow",
			"exhaust",
			"fluid",
			"hawkes",
			"leadlag",
			"liquidity",
			"manifold",
			"prediction",
			"pumpdump",
			"sentiment",
			"toxicity",
		)
	}

	return nil
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}

func (crypto *Crypto) updateSignals(
	artifact *datura.Artifact,
	signals ...string,
) error {
	for _, name := range signals {
		crypto.pool.ScheduleFast(func() {
			switch name {
			case "causal":
				signal, _ := crypto.signals.LoadOrStore(
					name, causal.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*causal.Signal).Update(artifact))
			case "correlation":
				signal, _ := crypto.signals.LoadOrStore(
					name, causal.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*causal.Signal).Update(artifact))
			case "cvd":
				signal, _ := crypto.signals.LoadOrStore(
					name, cvd.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*cvd.Signal).Update(artifact))
			case "depthflow":
				signal, _ := crypto.signals.LoadOrStore(
					name, depthflow.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*depthflow.Signal).Update(artifact))
			case "exhaust":
				signal, _ := crypto.signals.LoadOrStore(
					name, exhaust.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*exhaust.Signal).Update(artifact))
			case "fluid":
				signal, _ := crypto.signals.LoadOrStore(
					name, fluid.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*fluid.Signal).Update(artifact))
			case "hawkes":
				signal, _ := crypto.signals.LoadOrStore(
					name, hawkes.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*hawkes.Signal).Update(artifact))
			case "leadlag":
				signal, _ := crypto.signals.LoadOrStore(
					name, leadlag.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*leadlag.Signal).Update(artifact))
			case "liquidity":
				signal, _ := crypto.signals.LoadOrStore(
					name, liquidity.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*liquidity.Signal).Update(artifact))
			case "manifold":
				signal, _ := crypto.signals.LoadOrStore(
					name, causal.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*causal.Signal).Update(artifact))
			case "prediction":
				signal, _ := crypto.signals.LoadOrStore(
					name, prediction.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*prediction.Signal).Update(artifact))
			case "pumpdump":
				signal, _ := crypto.signals.LoadOrStore(
					name, pumpdump.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*pumpdump.Signal).Update(artifact))
			case "sentiment":
				signal, _ := crypto.signals.LoadOrStore(
					name, sentiment.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*sentiment.Signal).Update(artifact))
			case "toxicity":
				signal, _ := crypto.signals.LoadOrStore(
					name, toxicity.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*toxicity.Signal).Update(artifact))
			}
		})
	}

	return nil
}
