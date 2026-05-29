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

// moveClassifier is the only legitimate package-scope adaptive primitive
// here: Classifier carries no per-symbol state and its Next is read-only
// (it returns a label from immutable thresholds). The previous package-scope
// peakGate was a data race and a cross-symbol leakage path; it is now a
// per-symbol field on PumpSymbol.
var moveClassifier *adaptive.Classifier

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

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("pumpdump symbols channel closed"))
					return
				}

				for symbol, pair := range value.Value.(map[string]*asset.Pair) {
					if pair == nil {
						continue
					}

					state := NewPumpSymbol(*pair)
					pumpdump.symbols.Store(symbol, state)

					// The hourly-volume baseline is warmed from the live Kraken
					// REST API. In replay/backtest there is no live endpoint to
					// call -- and firing a burst of these per symbol drives
					// concurrent traffic through fiber's shared HTTP client, which
					// is not goroutine-safe and corrupts process memory. Skip the
					// warm under replay; SlowRVOL simply stays unready, which is the
					// correct behaviour for a feed that carries no REST history.
					if config.System.ReplayFile == "" {
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
				}

				pumpdump.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("pumpdump tick channel closed"))
					return
				}

				row := value.Value.(market.TickerRow)
				raw, ok := pumpdump.symbols.Load(row.Symbol)

				if ok && row.Last > 0 {
					state := raw.(*PumpSymbol)
					state.FeedTicker(row)

					if _, seen := pumpdump.requested.Load(row.Symbol); !seen {
						volumes := make([]float64, 0)

						pumpdump.symbols.Range(func(key, value any) bool {
							symbolState := value.(*PumpSymbol)

							if quoteVol := symbolState.DailyQuoteVol(); quoteVol > 0 {
								volumes = append(volumes, quoteVol)
							}

							return true
						})

						if len(volumes) >= 2 {
							median := numeric.PercentileSorted(numeric.CopySorted(volumes), 0.5)

							if state.DailyQuoteVol() >= median {
								pumpdump.requested.Store(row.Symbol, struct{}{})
								pumpdump.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{row.Symbol}})
								pumpdump.publishPulse()
							}
						}
					}
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["trade"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("pumpdump trade channel closed"))
					return
				}

				tick := value.Value.(trade.Data)
				raw, ok := pumpdump.symbols.Load(tick.Symbol)

				if ok {
					state := raw.(*PumpSymbol)
					state.FeedTrade(tick)

					if _, seen := pumpdump.requested.Load(tick.Symbol); !seen {
						if state.HasVolumeLift() {
							pumpdump.requested.Store(tick.Symbol, struct{}{})
							pumpdump.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{tick.Symbol}})
						}
					}

					if tick.Side == "sell" {
						pumpdump.publishPulse()
					}
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["book"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("pumpdump book channel closed"))
					return
				}

				delta := value.Value.(market.BookLevelsDelta)
				raw, ok := pumpdump.symbols.Load(delta.Symbol)

				if ok && len(delta.Bids) > 0 && len(delta.Asks) > 0 {
					state := raw.(*PumpSymbol)
					state.FeedBook(delta)

					pumpdump.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case value, ok := <-pumpdump.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("pumpdump feedback channel closed"))
					return
				}

				pumpdump.Feedback(value.Value.(engine.PredictionFeedback))
				pumpdump.publishPulse()
			}
		}
	})

	wg.Wait()
	return pumpdump.ctx.Err()
}

func (pumpdump *PumpDump) publishPulse() {
	pumpdump.publishMeasurements()
}

func (pumpdump *PumpDump) publishMeasurements() {
	spikes := make(map[string]float64)
	spikeRegimes := make(map[string]string)
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
			rvol := state.SlowRVOL()

			if state.HourlyBaselineReady() && rvol >= config.System.SlowRVOLThreshold {
				spikeRegimes[symbol] = "slow_breakout"
				slowRVOLs[symbol] = rvol
			}

			return true
		}

		if spike <= 1 {
			return true
		}

		spikeRegimes[symbol] = regime
		spikes[symbol] = spike

		return true
	})

	for symbol, spike := range spikes {
		raw, ok := pumpdump.symbols.Load(symbol)

		if !ok {
			continue
		}

		state := raw.(*PumpSymbol)
		regime := spikeRegimes[symbol]
		peakSpike, err := state.PeakSpike(spike, adaptive.PeerValues(spikes, symbol)...)

		if err != nil {
			errnie.Error(err)
			continue
		}

		if peakSpike <= 0 && regime != "slow_breakout" {
			continue
		}

		measurement, ok := state.Measure(peakSpike, regime)

		if !ok {
			continue
		}

		pumpdump.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}

	for symbol, rvol := range slowRVOLs {
		raw, ok := pumpdump.symbols.Load(symbol)

		if !ok {
			continue
		}

		state := raw.(*PumpSymbol)
		regime := spikeRegimes[symbol]
		measurement, ok := state.Measure(rvol, regime)

		if !ok {
			continue
		}

		pumpdump.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
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
			peakSpike, err := state.PeakSpike(spike, adaptive.PeerValues(spikes, symbol)...)

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

	state.ApplyFeedback(feedback)
}
