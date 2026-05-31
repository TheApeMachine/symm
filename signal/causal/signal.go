package causal

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
)

/*
Signal scores Pearl's ladder — association, intervention (backdoor-adjusted),
counterfactual uplift — over a DAG of MacroMomentum → PriceVelocity ← LocalFlow
with Liquidity as a backdoor control, switching to a panic regime when
cross-asset contagion or collinearity spikes. It consumes trades (flow), ticks
(macro change), and book (liquidity); the heavy fit lives in CausalSymbol.
*/
// causalPublishInterval throttles the cross-sectional causal fit. The fit is
// O(symbols²) (each symbol's reading depends on the macro median of the rest), so
// running it on every trade saturates a core; the structural picture does not
// change meaningfully faster than this.
const causalPublishInterval = 500 * time.Millisecond

type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	symbols       sync.Map
	lastPublishMu sync.Mutex
	lastPublish   time.Time
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func (signal *Signal) state(symbol string) *CausalSymbol {
	stored, _ := signal.symbols.LoadOrStore(symbol, NewCausalSymbol())
	return stored.(*CausalSymbol)
}

func (signal *Signal) Tick() error {
	symbols := config.System.Symbols
	trades := market.NewTradeSubscription(signal.ctx, symbols...)
	ticks := market.NewTickerSubscription(signal.ctx, symbols...)
	books := market.NewBookSubscription(signal.ctx, config.System.BookDepthLevels, symbols...)

	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case trade, ok := <-trades:
			if !ok {
				trades = nil
				continue
			}

			if trade != nil {
				signal.state(trade.Symbol).FeedTrade(*trade)
				signal.publish()
			}
		case row, ok := <-ticks:
			if !ok {
				ticks = nil
				continue
			}

			if row != nil {
				signal.state(row.Symbol).FeedTicker(*row)
			}
		case delta, ok := <-books:
			if !ok {
				books = nil
				continue
			}

			if delta != nil {
				signal.state(delta.Symbol).FeedBook(*delta)
			}
		}
	}
}

// throttle reports whether enough time has passed to rerun the O(symbols²) fit.
func (signal *Signal) throttle() bool {
	signal.lastPublishMu.Lock()
	defer signal.lastPublishMu.Unlock()

	if time.Since(signal.lastPublish) < causalPublishInterval {
		return false
	}

	signal.lastPublish = time.Now()

	return true
}

// publish runs the causal fit for every symbol against the current cross-asset
// macro momentum and contagion, emitting one structural reading each.
func (signal *Signal) publish() {
	if !signal.throttle() {
		return
	}

	now := time.Now()
	contagion := signal.contagion()

	signal.symbols.Range(func(key, value any) bool {
		state := value.(*CausalSymbol)
		macro := signal.macroMomentum(key.(string))

		measurement, ok := state.Measure(macro, contagion, now)

		if ok {
			measurement.Symbol = key.(string)
			stream := "macro"

			if measurement.Strength > 0 {
				stream = "intervention"
			}

			measurement = perspectives.FinalizeMeasurement(
				measurement,
				measurement.Strength,
				stream,
			)
			signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}

		return true
	})
}

/*
macroMomentum returns the median change_pct across every symbol other than
candidate. The candidate's own change is excluded so it cannot appear on both
sides of the structural regression (outcome and macro regressor), which would
inject contemporaneous self-correlation into the backdoor estimand.
*/
func (signal *Signal) macroMomentum(candidate string) float64 {
	changes := make([]float64, 0)

	signal.symbols.Range(func(key, value any) bool {
		if key.(string) == candidate {
			return true
		}

		if changePct := value.(*CausalSymbol).ChangePct(); changePct != 0 {
			changes = append(changes, changePct)
		}

		return true
	})

	if len(changes) < 2 {
		return 0
	}

	return numeric.PercentileSorted(numeric.CopySorted(changes), 0.5)
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
