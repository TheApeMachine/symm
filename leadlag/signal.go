package leadlag

import (
	"context"
	"iter"
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

type symbolState struct {
	pair      asset.Pair
	changePct float64
}

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
	select {
	case <-leadlag.ctx.Done():
		return leadlag.ctx.Err()
	case value := <-leadlag.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair == nil {
				continue
			}

			leadlag.symbols.Store(symbol, &symbolState{pair: *pair})

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
	case value := <-leadlag.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		raw, ok := leadlag.symbols.Load(row.Symbol)

		if !ok || row.ChangePct == 0 {
			break
		}

		state := raw.(*symbolState)
		state.changePct = row.ChangePct
		leadlag.publishPulse()
	case value := <-leadlag.subscribers["feedback"].Incoming:
		leadlag.Feedback(value.Value.(engine.PredictionFeedback))
		leadlag.publishPulse()
	default:
	}

	return nil
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

	if anchor.changePct < minAnchorMove {
		return
	}

	lags := leadlag.buildLags(anchor)
	anchorStrength := engine.ExcessRatio(anchor.changePct / minAnchorMove)
	waiters := make([]chan *qpool.QValue[any], 0, len(lags))

	for symbol, lagRatio := range lags {
		raw, ok := leadlag.symbols.Load(symbol)

		if !ok {
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

				if peakLag <= 0 {
					return nil, nil
				}

				measurement, ok := lagMeasurement(state, peakLag, anchorStrength)

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

		if anchor.changePct < minAnchorMove {
			return
		}

		lags := leadlag.buildLags(anchor)
		anchorStrength := engine.ExcessRatio(anchor.changePct / minAnchorMove)

		for symbol, lagRatio := range lags {
			peakLag, err := leadlag.peak.Next(lagRatio, adaptive.PeerValues(lags, symbol)...)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if peakLag <= 0 {
				continue
			}

			raw, ok := leadlag.symbols.Load(symbol)

			if !ok {
				continue
			}

			state := raw.(*symbolState)
			measurement, ok := lagMeasurement(state, peakLag, anchorStrength)

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
	if feedback.Source != leadlagSource {
		return
	}
}

func (leadlag *LeadLag) buildLags(anchor *symbolState) map[string]float64 {
	lags := make(map[string]float64)

	leadlag.symbols.Range(func(key, value any) bool {
		symbol := key.(string)
		state := value.(*symbolState)

		if symbol == anchorSymbol || state.changePct <= 0 {
			return true
		}

		lag := anchor.changePct - state.changePct

		if lag <= anchor.changePct*minLagFraction {
			return true
		}

		lags[symbol] = lag / anchor.changePct

		return true
	})

	return lags
}

func lagMeasurement(
	state *symbolState,
	peakLag float64,
	anchorStrength float64,
) (engine.Measurement, bool) {
	confidence := engine.AlignConfidence(peakLag, anchorStrength)

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
	}, true
}
