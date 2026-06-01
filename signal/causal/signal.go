package causal

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Signal scores Pearl's ladder — association, intervention (backdoor-adjusted),
counterfactual uplift — over a DAG of MacroMomentum → PriceVelocity ← LocalFlow
with Liquidity as a backdoor control, switching to a panic regime when
cross-asset contagion or collinearity spikes. It consumes trades (flow), ticks
(macro change), and book (liquidity); the heavy fit lives in CausalSymbol.

causalPublishInterval throttles the cross-sectional causal fit. The fit is
O(symbols²) (each symbol's reading depends on the macro median of the rest), so
running it on every trade saturates a core; the structural picture does not
change meaningfully faster than this.

| Category         | Active Regime | Dominant Factor       | Market "Feel"      |
|:-----------------|:--------------|:----------------------|:-------------------|
| Endogenous Alpha | Normal        | Counterfactual Uplift | Driven/Independent |
| Systemic Beta    | Normal        | Macro Momentum        | Drifting/Passive   |
| Liquidity Shock  | Panic         | Liquidity Void        | Fragile/Inverted   |
| Causal Noise     | Variable      | None                  | Stochastic/Unclear |
*/
const causalPublishInterval = 500 * time.Millisecond

type Signal struct {
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *qpool.Q
	broadcasts     map[string]*qpool.BroadcastGroup
	subscribers    map[string]*qpool.Subscriber
	symbols        sync.Map
	lastPublish    time.Time
	publishScratch []publishEntry
	floor          *adaptive.SNRField
}

type publishEntry struct {
	symbol string
	state  *CausalSymbol
	change float64
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		floor:       adaptive.NewSNRField(),
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func (signal *Signal) state(symbol string) *CausalSymbol {
	if stored, ok := signal.symbols.Load(symbol); ok {
		return stored.(*CausalSymbol)
	}

	created := NewCausalSymbol()
	stored, loaded := signal.symbols.LoadOrStore(symbol, created)

	if loaded {
		return stored.(*CausalSymbol)
	}

	return created
}

func (signal *Signal) Tick() error {
	symbols := viper.GetStringSlice("market.symbols")
	trades := market.NewTradeSubscription(signal.ctx, symbols...)
	ticks := market.NewTickerSubscription(signal.ctx, symbols...)
	books := market.NewBookSubscription(signal.ctx, viper.GetInt("market.book_depth_levels"), symbols...)

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
	entries := signal.snapshotEntries()
	macros := macroMedians(entries)

	for _, entry := range entries {
		measurement, ok := entry.state.Measure(macros[entry.symbol], contagion, now)

		if ok {
			measurement.Symbol = entry.symbol
			measurement.SNR = signal.floor.Score(measurement.Symbol, measurement.Confidence)
			signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}
	}
}

func (signal *Signal) snapshotEntries() []publishEntry {
	entries := signal.publishScratch[:0]

	signal.symbols.Range(func(key, value any) bool {
		state := value.(*CausalSymbol)
		entries = append(entries, publishEntry{
			symbol: key.(string),
			state:  state,
			change: state.ChangePct(),
		})

		return true
	})

	signal.publishScratch = entries

	return entries
}

func macroMedians(entries []publishEntry) map[string]float64 {
	macros := make(map[string]float64, len(entries))
	changes := make([]float64, 0, len(entries))

	for _, entry := range entries {
		if entry.change != 0 {
			changes = append(changes, entry.change)
		}
	}

	for entryIndex, candidate := range entries {
		peerChanges := changes[:0]

		for peerIndex, peer := range entries {
			if peerIndex == entryIndex || peer.change == 0 {
				continue
			}

			peerChanges = append(peerChanges, peer.change)
		}

		if len(peerChanges) < 2 {
			continue
		}

		macros[candidate.symbol] = numeric.PercentileSorted(
			numeric.CopySorted(peerChanges),
			0.5,
		)
	}

	return macros
}

/*
macroMomentum returns the median change_pct across every symbol other than
candidate. The candidate's own change is excluded so it cannot appear on both
sides of the structural regression (outcome and macro regressor), which would
inject contemporaneous self-correlation into the backdoor estimand.
*/
func (signal *Signal) macroMomentum(candidate string) float64 {
	entries := signal.snapshotEntries()
	macros := macroMedians(entries)

	return macros[candidate]
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
