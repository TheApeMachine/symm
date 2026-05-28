package leadlag

import (
	"context"
	"fmt"
	"iter"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	leadlagSource  = "leadlag"
	anchorSymbol   = "BTC/EUR"
	minAnchorMove  = 0.05
	minLagFraction = 0.35
)

/*
LeadLag detects altcoins lagging a moving anchor pair.
*/
type LeadLag struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	peak        *adaptive.Peak
	pending     []string
	requested   sync.Map
}

func NewLeadLag(ctx context.Context, pool *qpool.Q) *LeadLag {
	ctx, cancel := context.WithCancel(ctx)

	leadlag := &LeadLag{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		peak:        adaptive.NewPeak(),
		requested:   sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		leadlag.subscribers[channel] = group.Subscribe("leadlag:"+channel, 128)
	}

	leadlag.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	leadlag.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return leadlag
}

func (leadlag *LeadLag) Start() error        { return nil }
func (leadlag *LeadLag) State() engine.State { return engine.READY }

func (leadlag *LeadLag) Tick() error {
	errnie.Info("starting leadlag tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-leadlag.ctx.Done():
				return
			case value, ok := <-leadlag.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("leadlag symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("leadlag: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
					if pair == nil {
						continue
					}

					leadlag.symbols.Store(symbol, newSymbolState(*pair))

					if pair.Quote != config.System.QuoteCurrency {
						continue
					}

					if _, seen := leadlag.requested.Load(symbol); seen {
						continue
					}

					if symbol == anchorSymbol {
						leadlag.requested.Store(symbol, struct{}{})
						leadlag.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{symbol}})
						continue
					}

					leadlag.pending = append(leadlag.pending, symbol)
				}

				leadlag.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-leadlag.ctx.Done():
				return
			case value, ok := <-leadlag.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("leadlag tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("leadlag: invalid ticker payload: %T", value.Value))
					continue
				}

				raw, ok := leadlag.symbols.Load(row.Symbol)

				if ok && row.ChangePct != 0 {
					state := raw.(*symbolState)
					state.changePct = row.ChangePct

					if row.Last > 0 {
						state.last = row.Last
					}

					if row.Bid > 0 {
						state.bid = row.Bid
					}

					if row.Ask > 0 {
						state.ask = row.Ask
					}

					leadlag.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-leadlag.ctx.Done():
				return
			case value, ok := <-leadlag.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("leadlag feedback channel closed"))
					return
				}

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("leadlag: invalid feedback payload: %T", value.Value))
					continue
				}

				leadlag.Feedback(fb)
				leadlag.publishPulse()
			}
		}
	})

	wg.Wait()
	return leadlag.ctx.Err()
}

func (leadlag *LeadLag) requestedCount() int {
	count := 0

	leadlag.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (leadlag *LeadLag) publishPulse() {
	anchorRaw, ok := leadlag.symbols.Load(anchorSymbol)

	if ok {
		anchor := anchorRaw.(*symbolState)

		if anchor.changePct >= minAnchorMove {
			scanCap := max(config.System.MaxScanSymbols/8, 1)
			requested := leadlag.requestedCount()

			if len(leadlag.pending) > 0 && requested < scanCap {
				remaining := scanCap - requested
				batch := min(min(config.System.SubscribeBatch, remaining), len(leadlag.pending))

				symbols := leadlag.pending[:batch]
				leadlag.pending = leadlag.pending[batch:]

				for _, symbol := range symbols {
					leadlag.requested.Store(symbol, struct{}{})
				}

				leadlag.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
			}
		}
	}

	leadlag.publishMeasurements()
}

func (leadlag *LeadLag) publishMeasurements() {
	anchorRaw, ok := leadlag.symbols.Load(anchorSymbol)

	if !ok {
		return
	}

	anchor := anchorRaw.(*symbolState)
	lags := leadlag.lagRatios(anchor)
	waiters := make([]chan *qpool.QValue[any], 0, len(lags))

	for symbol, lagRatio := range lags {
		if _, subscribed := leadlag.requested.Load(symbol); !subscribed {
			continue
		}

		raw, loaded := leadlag.symbols.Load(symbol)

		if !loaded {
			continue
		}

		state := raw.(*symbolState)
		waiters = append(
			waiters,
			leadlag.pool.ScheduleFast(leadlag.ctx, func(ctx context.Context) (any, error) {
				peakLag, err := leadlag.peak.Next(lagRatio, adaptive.PeerValues(lags, symbol)...)

				if err != nil {
					return nil, err
				}

				measurement, ok := lagMeasurement(anchor, state, peakLag, lagRatio)

				if !ok {
					return nil, nil
				}

				return measurement, nil
			}),
		)
	}

	for _, waiter := range waiters {
		value := <-waiter

		if value == nil {
			continue
		}

		if value.Error != nil {
			errnie.Error(value.Error)
			continue
		}

		measurement, ok := value.Value.(engine.Measurement)

		if !ok {
			continue
		}

		leadlag.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (leadlag *LeadLag) Close() error {
	leadlag.cancel()
	return nil
}

func (leadlag *LeadLag) Source() string { return leadlagSource }

func (leadlag *LeadLag) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		anchorRaw, ok := leadlag.symbols.Load(anchorSymbol)

		if !ok {
			return
		}

		anchor := anchorRaw.(*symbolState)
		lags := leadlag.lagRatios(anchor)

		for symbol, lagRatio := range lags {
			peakLag, err := leadlag.peak.Next(lagRatio, adaptive.PeerValues(lags, symbol)...)

			if err != nil {
				errnie.Error(err)
				continue
			}

			raw, loaded := leadlag.symbols.Load(symbol)

			if !loaded {
				continue
			}

			state := raw.(*symbolState)
			measurement, ok := lagMeasurement(anchor, state, peakLag, lagRatio)

			if !ok {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (leadlag *LeadLag) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, leadlagSource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := leadlag.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*symbolState)

	if _, err := state.forecastLearner().Next(
		0, feedback.PredictedReturn, feedback.ActualReturn,
	); err != nil {
		errnie.Error(err)
	}
}

func (leadlag *LeadLag) lagRatios(anchor *symbolState) map[string]float64 {
	ratios := make(map[string]float64)

	leadlag.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if symbol == anchorSymbol {
			return true
		}

		state := value.(*symbolState)

		if state.changePct == 0 && anchor.changePct == 0 {
			return true
		}

		gap := anchor.changePct - state.changePct

		if anchor.changePct > 0 && gap > anchor.changePct*minLagFraction {
			ratios[symbol] = gap / anchor.changePct

			return true
		}

		ratios[symbol] = math.Abs(gap)

		return true
	})

	return ratios
}

func lagMeasurement(
	anchor *symbolState,
	state *symbolState,
	peakLag float64,
	lagRatio float64,
) (engine.Measurement, bool) {
	anchorStrength := engine.ExcessRatio(anchor.changePct / minAnchorMove)
	scale := state.forecastScale()
	score := peakLag * scale

	if score <= 0 {
		score = lagRatio * scale
	}

	confidence := engine.AlignConfidence(score, anchorStrength)
	fmt.Println("confidence", confidence)

	if confidence <= 0 {
		confidence = engine.ProvisionalConfidence(
			0,
			math.Abs(anchor.changePct-state.changePct)*scale,
		)
	}

	if confidence <= 0 && state.changePct != 0 {
		confidence = engine.ConfidenceFromScore(math.Abs(state.changePct) * scale)
	}

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.LeadLag,
		Source:     leadlagSource,
		Regime:     "cross_asset",
		Reason:     "anchor_lag",
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       state.last,
		Bid:        state.bid,
		Ask:        state.ask,
	}, true
}
