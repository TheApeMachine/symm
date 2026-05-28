package pumpdump

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
	"github.com/theapemachine/symm/kraken/trade"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const pumpdumpSource = "pumpdump"

var (
	moveClassifier *adaptive.Classifier
	peakGate       = adaptive.NewPeak()
)

func init() {
	var err error

	moveClassifier, err = adaptive.NewClassifier(
		[]float64{-0.001, 0.001},
		[]float64{0, 1, 2},
		[]string{"dump", "precursor", "actual_pump"},
	)

	if err != nil {
		errnie.Error(err)
	}
}

/*
PumpDump detects pre-pump microstructure from Kraken book, trade, and ticker streams.
*/
type PumpDump struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	requested   sync.Map
}

/*
NewPumpDump wires broadcast subscribers for the pumpdump system.
*/
func NewPumpDump(ctx context.Context, pool *qpool.Q) *PumpDump {
	ctx, cancel := context.WithCancel(ctx)

	pumpdump := &PumpDump{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		requested:   sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "trade", "book", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		pumpdump.subscribers[channel] = group.Subscribe("pumpdump:"+channel, 128)
	}

	pumpdump.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	pumpdump.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return pumpdump
}

func (pumpdump *PumpDump) Start() error {
	return nil
}

func (pumpdump *PumpDump) State() engine.State {
	return engine.READY
}

func (pumpdump *PumpDump) Tick() error {
	errnie.Info("starting pumpdump tick")

	var workers sync.WaitGroup
	errs := make(chan error, 1)
	fail := func(err error) {
		select {
		case errs <- err:
			pumpdump.cancel()
		default:
		}
	}

	workers.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["symbols"].Incoming:
				if !ok {
					fail(fmt.Errorf("pumpdump symbols channel closed"))
					return
				}

				pumpdump.mu.Lock()
				for symbol, pair := range value.Value.(map[string]*asset.Pair) {
					if pair == nil {
						continue
					}

					state := NewPumpSymbol(*pair)
					pumpdump.symbols.Store(symbol, state)

					pumpdump.pool.ScheduleFast(pumpdump.ctx, func(ctx context.Context) (any, error) {
						median, err := WarmHourlyVolumeBaseline(*pair)

						if err != nil {
							if err != ErrNoVolumeData {
								errnie.Error(err)
							}

							return nil, err
						}

						state.SetMedianHourlyVolume(median)

						return nil, nil
					})
				}

				pumpdump.publishPulse()
				pumpdump.mu.Unlock()
			}
		}
	})

	workers.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["tick"].Incoming:
				if !ok {
					fail(fmt.Errorf("pumpdump tick channel closed"))
					return
				}

				pumpdump.mu.Lock()
				row := value.Value.(market.TickerRow)
				raw, ok := pumpdump.symbols.Load(row.Symbol)

				if ok && row.Last > 0 {
					state := raw.(*PumpSymbol)
					state.lastPrice = row.Last
					state.dailyQuoteVol = row.Volume * row.Last

					if row.Bid > 0 {
						state.bid = row.Bid
					}

					if row.Ask > 0 {
						state.ask = row.Ask
					}

					if _, seen := pumpdump.requested.Load(row.Symbol); !seen {
						volumes := make([]float64, 0)

						pumpdump.symbols.Range(func(key, value any) bool {
							symbolState := value.(*PumpSymbol)

							if symbolState.dailyQuoteVol > 0 {
								volumes = append(volumes, symbolState.dailyQuoteVol)
							}

							return true
						})

						if len(volumes) >= 2 {
							median := numeric.PercentileSorted(numeric.CopySorted(volumes), 0.5)

							if state.dailyQuoteVol >= median {
								pumpdump.requested.Store(row.Symbol, struct{}{})
								pumpdump.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{row.Symbol}})
								pumpdump.publishPulse()
							}
						}
					}
				}

				pumpdump.mu.Unlock()
			}
		}
	})

	workers.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["trade"].Incoming:
				if !ok {
					fail(fmt.Errorf("pumpdump trade channel closed"))
					return
				}

				pumpdump.mu.Lock()
				tick := value.Value.(trade.Data)
				raw, ok := pumpdump.symbols.Load(tick.Symbol)

				if ok {
					state := raw.(*PumpSymbol)
					state.FeedTradeVolume(tick.Timestamp, tick.Qty, state.lastPrice)

					if _, seen := pumpdump.requested.Load(tick.Symbol); !seen {
						fastBaseline := state.fastVolumeBaseline.Value()
						mediumBaseline := state.mediumVolumeBaseline.Value()

						if fastBaseline > 0 && mediumBaseline > 0 &&
							(state.fastVolumeWindow.Sum() > fastBaseline ||
								state.mediumVolumeWindow.Sum() > mediumBaseline) {
							pumpdump.requested.Store(tick.Symbol, struct{}{})
							pumpdump.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{tick.Symbol}})
						}
					}

					if tick.Side == "buy" {
						state.buyPressure = 1
					}

					if tick.Side == "sell" {
						state.buyPressure = -1
						pumpdump.publishPulse()
					}
				}

				pumpdump.mu.Unlock()
			}
		}
	})

	workers.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["book"].Incoming:
				if !ok {
					fail(fmt.Errorf("pumpdump book channel closed"))
					return
				}

				pumpdump.mu.Lock()
				delta := value.Value.(market.BookLevelsDelta)
				raw, ok := pumpdump.symbols.Load(delta.Symbol)

				if ok && len(delta.Bids) > 0 && len(delta.Asks) > 0 {
					state := raw.(*PumpSymbol)
					bid := delta.Bids[0].Price
					ask := delta.Asks[0].Price
					mid := (bid + ask) / 2

					state.bid = bid
					state.ask = ask

					if state.lastPrice <= 0 && mid > 0 {
						state.lastPrice = mid
					}

					if mid > 0 {
						state.spreadBPS = (ask - bid) / mid * 10000
					}

					total := delta.Bids[0].Volume + delta.Asks[0].Volume

					if total > 0 {
						state.imbalance = (delta.Bids[0].Volume - delta.Asks[0].Volume) / total
					}

					pumpdump.publishPulse()
				}

				pumpdump.mu.Unlock()
			}
		}
	})

	workers.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["feedback"].Incoming:
				if !ok {
					fail(fmt.Errorf("pumpdump feedback channel closed"))
					return
				}

				pumpdump.mu.Lock()
				pumpdump.Feedback(value.Value.(engine.PredictionFeedback))
				pumpdump.publishPulse()
				pumpdump.mu.Unlock()
			}
		}
	})

	done := make(chan struct{})

	go func() {
		workers.Wait()
		close(done)
	}()

	select {
	case err := <-errs:
		workers.Wait()
		return errnie.Error(err)
	case <-pumpdump.ctx.Done():
		workers.Wait()
		return pumpdump.ctx.Err()
	case <-done:
		return pumpdump.ctx.Err()
	}
}

func (pumpdump *PumpDump) publishPulse() {
	pumpdump.publishMeasurements()
}

func (pumpdump *PumpDump) publishMeasurements() {
	spikeWaiters := make([]chan *qpool.QValue[any], 0)

	pumpdump.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := pumpdump.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*PumpSymbol)
		spikeWaiters = append(
			spikeWaiters,
			pumpdump.pool.ScheduleFast(pumpdump.ctx, func(ctx context.Context) (any, error) {
				spike, regime, err := state.BestVolumeSpike()

				if err != nil {
					return nil, err
				}

				if spike <= 1 && regime == "" {
					if state.HourlyBaselineReady() &&
						state.SlowRVOL() >= config.System.SlowRVOLThreshold {
						return spikeEntry{
							symbol: symbol,
							rvol:   state.SlowRVOL(),
							regime: "slow_breakout",
						}, nil
					}

					return nil, nil
				}

				if spike <= 1 {
					return nil, nil
				}

				return spikeEntry{symbol: symbol, spike: spike, regime: regime}, nil
			}),
		)

		return true
	})

	spikes := make(map[string]float64)
	spikeRegimes := make(map[string]string)
	slowRVOLs := make(map[string]float64)

	for _, waiter := range spikeWaiters {
		value := <-waiter

		if value == nil {
			continue
		}

		if value.Error != nil {
			errnie.Error(value.Error)
			continue
		}

		entry, ok := value.Value.(spikeEntry)

		if !ok {
			continue
		}

		spikeRegimes[entry.symbol] = entry.regime

		if entry.regime == "slow_breakout" {
			slowRVOLs[entry.symbol] = entry.rvol
			continue
		}

		spikes[entry.symbol] = entry.spike
	}

	measureWaiters := make([]chan *qpool.QValue[any], 0)

	for symbol, spike := range spikes {
		raw, ok := pumpdump.symbols.Load(symbol)

		if !ok {
			continue
		}

		state := raw.(*PumpSymbol)
		regime := spikeRegimes[symbol]
		measureWaiters = append(
			measureWaiters,
			pumpdump.pool.ScheduleFast(pumpdump.ctx, func(ctx context.Context) (any, error) {
				peakSpike, err := peakGate.Next(spike, adaptive.PeerValues(spikes, symbol)...)

				if err != nil {
					return nil, err
				}

				if peakSpike <= 0 && regime != "slow_breakout" {
					return nil, nil
				}

				// Slow breakouts skip peer peak gating: RVOL is a gradual regime
				// shift, not an instantaneous cross-section spike; other regimes
				// still require peakSpike > 0 from adaptive.PeerValues.
				if regime == "slow_breakout" {
					peakSpike = slowRVOLs[symbol]
				}

				measurement, ok := state.Measure(peakSpike, regime)

				if !ok {
					return nil, nil
				}

				return measurement, nil
			}),
		)
	}

	for symbol, rvol := range slowRVOLs {
		raw, ok := pumpdump.symbols.Load(symbol)

		if !ok {
			continue
		}

		state := raw.(*PumpSymbol)
		regime := spikeRegimes[symbol]
		measureWaiters = append(
			measureWaiters,
			pumpdump.pool.ScheduleFast(pumpdump.ctx, func(ctx context.Context) (any, error) {
				// Slow breakouts skip peer peak gating (see comment above).
				measurement, ok := state.Measure(rvol, regime)

				if !ok {
					return nil, nil
				}

				return measurement, nil
			}),
		)
	}

	for _, waiter := range measureWaiters {
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

		pumpdump.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

type spikeEntry struct {
	symbol string
	spike  float64 // adaptive.Ratio volume spike (fast/medium)
	rvol   float64 // 14d-median RVOL (slow_breakout only)
	regime string
}

func (pumpdump *PumpDump) Close() error {
	pumpdump.cancel()

	return nil
}

func (pumpdump *PumpDump) Source() string {
	return pumpdumpSource
}

func (pumpdump *PumpDump) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		spikes := make(map[string]float64)
		regimes := make(map[string]string)
		slowRVOLs := make(map[string]float64)

		pumpdump.symbols.Range(func(key, value any) bool {
			symbol := key.(string)

			if _, subscribed := pumpdump.requested.Load(symbol); !subscribed {
				return true
			}

			state := value.(*PumpSymbol)
			spike, regime, err := state.BestVolumeSpike()

			if err != nil {
				errnie.Error(err)
				return true
			}

			if spike <= 1 && regime == "" {
				if state.HourlyBaselineReady() &&
					state.SlowRVOL() >= config.System.SlowRVOLThreshold {
					slowRVOLs[symbol] = state.SlowRVOL()
					regimes[symbol] = "slow_breakout"
				}

				return true
			}

			if spike <= 1 {
				return true
			}

			spikes[symbol] = spike
			regimes[symbol] = regime

			return true
		})

		for symbol, spike := range spikes {
			raw, ok := pumpdump.symbols.Load(symbol)

			if !ok {
				continue
			}

			state := raw.(*PumpSymbol)
			regime := regimes[symbol]
			peakSpike, err := peakGate.Next(spike, adaptive.PeerValues(spikes, symbol)...)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if peakSpike <= 0 && regime != "slow_breakout" {
				continue
			}

			// Slow breakouts skip peer peak gating (see publishMeasurements).
			if regime == "slow_breakout" {
				peakSpike = slowRVOLs[symbol]
			}

			measurement, ok := state.Measure(peakSpike, regime)

			if !ok {
				continue
			}

			if !yield(measurement) {
				return
			}
		}

		for symbol, rvol := range slowRVOLs {
			raw, ok := pumpdump.symbols.Load(symbol)

			if !ok {
				continue
			}

			state := raw.(*PumpSymbol)
			regime := regimes[symbol]
			measurement, ok := state.Measure(rvol, regime)

			if !ok {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (pumpdump *PumpDump) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, pumpdumpSource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := pumpdump.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*PumpSymbol)

	if _, err := state.forecast.Next(
		0, feedback.PredictedReturn, feedback.ActualReturn,
	); err != nil {
		errnie.Error(err)
	}
}
