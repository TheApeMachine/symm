package causal

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
	"github.com/theapemachine/symm/numeric"
)

const causalSource = "causal"

const minLiquidityPairs = 2

/*
Causal scores Pearl's ladder: association, intervention, counterfactual uplift.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, with Liquidity as backdoor control.
*/
type Causal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     map[string]*CausalSymbol
	calibration engine.CalibrationParams
}

var (
	_ engine.System = (*Causal)(nil)
	_ engine.Signal = (*Causal)(nil)
)

func NewCausal(ctx context.Context, pool *qpool.Q) *Causal {
	ctx, cancel := context.WithCancel(ctx)

	causal := &Causal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     make(map[string]*CausalSymbol),
		calibration: engine.DefaultCalibrationParams(),
	}

	for _, channel := range []string{"symbols", "tick", "trade", "book", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		causal.subscribers[channel] = group.Subscribe("causal:"+channel, 128)
	}

	causal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return causal
}

func (causal *Causal) Start() error  { return nil }
func (causal *Causal) State() engine.State { return engine.READY }

func (causal *Causal) Tick() error {
	select {
	case <-causal.ctx.Done():
		return causal.ctx.Err()
	case value := <-causal.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair != nil {
				causal.symbols[symbol] = NewCausalSymbol(*pair, causal.calibration)
			}
		}
	case value := <-causal.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		state := causal.symbols[row.Symbol]

		if state == nil {
			return nil
		}

		state.FeedTicker(row)
	case value := <-causal.subscribers["trade"].Incoming:
		tick := value.Value.(trade.Data)
		state := causal.symbols[tick.Symbol]

		if state == nil {
			return nil
		}

		state.FeedTrade(tick)
	case value := <-causal.subscribers["book"].Incoming:
		delta := value.Value.(market.BookLevelsDelta)
		state := causal.symbols[delta.Symbol]

		if state == nil {
			return nil
		}

		state.FeedBook(delta)
	case value := <-causal.subscribers["feedback"].Incoming:
		causal.Feedback(value.Value.(engine.PredictionFeedback))
	default:
		for measurement := range causal.Measure() {
			causal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}
	}

	return nil
}

func (causal *Causal) Close() error {
	causal.cancel()
	return nil
}

func (causal *Causal) Source() string { return causalSource }

func (causal *Causal) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		macro := causal.macroMomentum()
		now := time.Now()

		for symbol, state := range causal.symbols {
			if !causal.passesLiquidity(symbol) {
				continue
			}

			measurement, ok := state.Measure(macro, now)

			if !ok {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (causal *Causal) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != causalSource || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	state := causal.symbols[feedback.Symbol]

	if state == nil {
		return
	}

	state.ApplyFeedback(feedback)
}

func (causal *Causal) macroMomentum() float64 {
	changes := make([]float64, 0, len(causal.symbols))

	for _, state := range causal.symbols {
		if state.changePct != 0 {
			changes = append(changes, state.changePct)
		}
	}

	if len(changes) == 0 {
		return 0
	}

	return numeric.PercentileSorted(numeric.CopySorted(changes), 0.5)
}

func (causal *Causal) passesLiquidity(symbol string) bool {
	state := causal.symbols[symbol]

	if state == nil {
		return false
	}

	quotes := make(map[string]float64, len(causal.symbols))

	for name, candidate := range causal.symbols {
		if candidate.dailyQuoteVol > 0 {
			quotes[name] = candidate.dailyQuoteVol
		}
	}

	return engine.PassesBelowMedianLiquidity(
		state.dailyQuoteVol,
		quotes,
		symbol,
		minLiquidityPairs,
	)
}
