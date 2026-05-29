package correlation

import (
	"context"
	"fmt"
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

const minCorrelationPeers = 2

/*
Signal measures synchronized return correlation across subscribed symbols.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	peakMu      sync.Mutex
	peak        *adaptive.Peak
	pending     []string
	requested   sync.Map
	windowCap   int
	minSamples  int
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		peak:        adaptive.NewPeak(),
		requested:   sync.Map{},
		windowCap:   windowCap(),
		minSamples:  config.System.MinCorrelationSamples,
	}

	for _, channel := range []string{"symbols", "tick", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = group.Subscribe("correlation:"+channel, 128)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return signal
}

func (signal *Signal) Start() error        { return nil }
func (signal *Signal) State() engine.State { return engine.READY }

func (signal *Signal) Tick() error {
	errnie.Info("starting correlation tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case value, ok := <-signal.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("correlation symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("signal: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
					if pair == nil {
						continue
					}

					signal.symbols.Store(symbol, newSymbolState(*pair, signal.windowCap))

					if pair.Quote != config.System.QuoteCurrency {
						continue
					}

					if _, seen := signal.requested.Load(symbol); seen {
						continue
					}

					signal.pending = append(signal.pending, symbol)
				}

				signal.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case value, ok := <-signal.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("correlation tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("signal: invalid ticker payload: %T", value.Value))
					continue
				}

				raw, ok := signal.symbols.Load(row.Symbol)

				if ok && row.Last > 0 {
					state := raw.(*symbolState)
					state.observeTick(row, time.Now())
					signal.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case value, ok := <-signal.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("correlation feedback channel closed"))
					return
				}

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("signal: invalid feedback payload: %T", value.Value))
					continue
				}

				signal.Feedback(fb)
				signal.publishPulse()
			}
		}
	})

	wg.Wait()
	return signal.ctx.Err()
}

func (signal *Signal) requestedCount() int {
	count := 0

	signal.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (signal *Signal) publishPulse() {
	scanCap := max(config.System.MaxScanSymbols/8, minCorrelationPeers)
	requested := signal.requestedCount()

	if len(signal.pending) > 0 && requested < scanCap {
		remaining := scanCap - requested
		batch := min(min(config.System.SubscribeBatch, remaining), len(signal.pending))

		symbols := signal.pending[:batch]
		signal.pending = signal.pending[batch:]

		for _, symbol := range symbols {
			signal.requested.Store(symbol, struct{}{})
		}

		signal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	signal.publishMeasurements()
}

func (signal *Signal) publishMeasurements() {
	correlations := signal.collectCorrelations()
	waiters := make([]chan *qpool.QValue[any], 0, len(correlations))

	for symbol, correlation := range correlations {
		if _, subscribed := signal.requested.Load(symbol); !subscribed {
			continue
		}

		raw, loaded := signal.symbols.Load(symbol)

		if !loaded {
			continue
		}

		state := raw.(*symbolState)
		waiters = append(
			waiters,
			signal.pool.ScheduleFast(signal.ctx, func(ctx context.Context) (any, error) {
				signal.peakMu.Lock()
				peakScore, err := signal.peak.Next(
					correlation, adaptive.PeerValues(correlations, symbol)...,
				)
				signal.peakMu.Unlock()

				if err != nil {
					return nil, err
				}

				measurement, ok := correlationMeasurement(state, peakScore, correlation)

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

		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (signal *Signal) collectCorrelations() map[string]float64 {
	subscribed := signal.subscribedStates()

	if len(subscribed) < minCorrelationPeers {
		return nil
	}

	correlations := make(map[string]float64, len(subscribed))

	for symbol, state := range subscribed {
		peak := 0.0

		for peerSymbol, peerState := range subscribed {
			if peerSymbol == symbol {
				continue
			}

			correlation, ok := pairCorrelation(state, peerState, signal.minSamples)

			if !ok {
				continue
			}

			if correlation > peak {
				peak = correlation
			}
		}

		if peak <= 0 {
			continue
		}

		correlations[symbol] = peak
	}

	return correlations
}

func (signal *Signal) subscribedStates() map[string]*symbolState {
	states := make(map[string]*symbolState)

	signal.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := signal.requested.Load(symbol); !subscribed {
			return true
		}

		states[symbol] = value.(*symbolState)

		return true
	})

	return states
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}

func (signal *Signal) Source() string { return correlationSource }

func (signal *Signal) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		correlations := signal.collectCorrelations()

		for symbol, correlation := range correlations {
			raw, ok := signal.symbols.Load(symbol)

			if !ok {
				continue
			}

			state := raw.(*symbolState)
			signal.peakMu.Lock()
			peakScore, err := signal.peak.Next(
				correlation, adaptive.PeerValues(correlations, symbol)...,
			)
			signal.peakMu.Unlock()

			if err != nil {
				errnie.Error(err)
				continue
			}

			measurement, ok := correlationMeasurement(state, peakScore, correlation)

			if !ok {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, correlationSource) ||
		feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := signal.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*symbolState)

	if err := state.applyFeedback(feedback.PredictedReturn, feedback.ActualReturn); err != nil {
		errnie.Error(err)
	}
}
