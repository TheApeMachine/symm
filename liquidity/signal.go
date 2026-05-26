package liquidity

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/learned"
)

const (
	liquiditySource   = "liquidity"
	minLiquidityPeers = 2
)

type symbolState struct {
	pair          asset.Pair
	dailyQuoteVol float64
	forecast      *learned.Forecast
}

/*
Liquidity ranks cross-section quote volume below the peer median.
*/
type Liquidity struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     map[string]*symbolState
	belowMedian *adaptive.BelowMedian
	peak        *adaptive.Peak
	pending     []string
	requested   map[string]struct{}
}

func NewLiquidity(ctx context.Context, pool *qpool.Q) *Liquidity {
	ctx, cancel := context.WithCancel(ctx)

	liquidity := &Liquidity{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     make(map[string]*symbolState),
		belowMedian: adaptive.NewBelowMedian(),
		peak:        adaptive.NewPeak(),
		requested:   make(map[string]struct{}),
	}

	for _, channel := range []string{"symbols", "tick", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		liquidity.subscribers[channel] = group.Subscribe("liquidity:"+channel, 128)
	}

	liquidity.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	liquidity.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return liquidity
}

func (liquidity *Liquidity) Start() error        { return nil }
func (liquidity *Liquidity) State() engine.State { return engine.READY }

func (liquidity *Liquidity) Tick() error {
	select {
	case <-liquidity.ctx.Done():
		return liquidity.ctx.Err()
	case value := <-liquidity.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair == nil {
				continue
			}

			liquidity.symbols[symbol] = &symbolState{
				pair:     *pair,
				forecast: learned.NewForecast(0),
			}

			if pair.Quote != config.System.QuoteCurrency {
				continue
			}

			if _, seen := liquidity.requested[symbol]; seen {
				continue
			}

			liquidity.pending = append(liquidity.pending, symbol)
		}
	case value := <-liquidity.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		state := liquidity.symbols[row.Symbol]

		if state == nil || row.Last <= 0 {
			break
		}

		state.dailyQuoteVol = row.Volume * row.Last
	case value := <-liquidity.subscribers["feedback"].Incoming:
		liquidity.Feedback(value.Value.(engine.PredictionFeedback))
	default:
	}

	liquidity.publishPulse()

	return nil
}

func (liquidity *Liquidity) publishPulse() {
	scanCap := config.System.MaxScanSymbols / 8

	if scanCap < 1 {
		scanCap = 1
	}

	if len(liquidity.pending) > 0 && len(liquidity.requested) < scanCap {
		remaining := scanCap - len(liquidity.requested)
		batch := config.System.SubscribeBatch

		if batch > remaining {
			batch = remaining
		}

		if batch > len(liquidity.pending) {
			batch = len(liquidity.pending)
		}

		symbols := liquidity.pending[:batch]
		liquidity.pending = liquidity.pending[batch:]

		for _, symbol := range symbols {
			liquidity.requested[symbol] = struct{}{}
		}

		liquidity.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	for measurement := range liquidity.Measure() {
		liquidity.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}
}

func (liquidity *Liquidity) Close() error {
	liquidity.cancel()
	return nil
}

func (liquidity *Liquidity) Source() string { return liquiditySource }

func (liquidity *Liquidity) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		quotes := make(map[string]float64, len(liquidity.symbols))

		for symbol, state := range liquidity.symbols {
			if state.dailyQuoteVol > 0 {
				quotes[symbol] = state.dailyQuoteVol
			}
		}

		candidates := make(map[string]float64, len(quotes))

		for symbol, quoteVol := range quotes {
			peers := adaptive.PeerValues(quotes, symbol)

			if len(peers) < minLiquidityPeers {
				continue
			}

			liquid, err := liquidity.belowMedian.Next(quoteVol, peers...)

			if err != nil || liquid <= 0 {
				continue
			}

			score := adaptive.IlliquidityScore(quoteVol, peers)

			if score <= 0 {
				continue
			}

			candidates[symbol] = score
		}

		for symbol, rawScore := range candidates {
			peakScore, err := liquidity.peak.Next(rawScore, adaptive.PeerValues(candidates, symbol)...)

			if err != nil || peakScore <= 0 {
				continue
			}

			state := liquidity.symbols[symbol]
			confidence := liquidityConfidence(peakScore, adaptive.PeerValues(candidates, symbol))

			if confidence <= 0 {
				continue
			}

			if !yield(engine.Measurement{
				Type:       engine.Liquidity,
				Source:     liquiditySource,
				Regime:     "liquidity",
				Reason:     "below_median",
				Pairs:      []asset.Pair{state.pair},
				Confidence: confidence,
			}) {
				return
			}
		}
	}
}

func (liquidity *Liquidity) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != liquiditySource || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	state := liquidity.symbols[feedback.Symbol]

	if state == nil {
		return
	}

	_, _ = state.forecast.Next(0, feedback.PredictedReturn, feedback.ActualReturn)
}
