package causal

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
	"github.com/theapemachine/symm/numeric"
)

const causalSource = "causal"

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
	symbols     sync.Map
	pending     []string
	requested   sync.Map
}

func NewCausal(ctx context.Context, pool *qpool.Q) *Causal {
	ctx, cancel := context.WithCancel(ctx)

	causal := &Causal{
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
		causal.subscribers[channel] = group.Subscribe("causal:"+channel, 128)
	}

	for _, channel := range []string{"measurements", "subscriptions"} {
		causal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	}

	return causal
}

func (causal *Causal) Start() error        { return nil }
func (causal *Causal) State() engine.State { return engine.READY }

func (causal *Causal) Tick() error {
	errnie.Info("starting causal tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-causal.ctx.Done():
				return
			case value, ok := <-causal.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("causal symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("causal: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
					if pair == nil {
						continue
					}

					causal.symbols.Store(symbol, NewCausalSymbol(*pair, causal.calibration))

					if pair.Quote != config.System.QuoteCurrency {
						continue
					}

					if _, seen := causal.requested.Load(symbol); seen {
						continue
					}

					causal.pending = append(causal.pending, symbol)
				}

				causal.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-causal.ctx.Done():
				return
			case value, ok := <-causal.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("causal tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("causal: invalid ticker payload: %T", value.Value))
					continue
				}

				raw, ok := causal.symbols.Load(row.Symbol)

				if ok {
					state := raw.(*CausalSymbol)
					state.FeedTicker(row)

					if _, seen := causal.requested.Load(row.Symbol); !seen && state.ChangePct() != 0 {
						causal.requested.Store(row.Symbol, struct{}{})
						causal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{row.Symbol}})
						causal.publishPulse()
					}
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-causal.ctx.Done():
				return
			case value, ok := <-causal.subscribers["trade"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("causal trade channel closed"))
					return
				}

				tick, tickOK := value.Value.(trade.Data)
				if !tickOK {
					errnie.Error(fmt.Errorf("causal: invalid trade payload: %T", value.Value))
					continue
				}

				raw, ok := causal.symbols.Load(tick.Symbol)

				if ok {
					state := raw.(*CausalSymbol)
					state.FeedTrade(tick)

					if _, seen := causal.requested.Load(tick.Symbol); !seen {
						causal.requested.Store(tick.Symbol, struct{}{})
						causal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{tick.Symbol}})
						causal.publishPulse()
					}
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-causal.ctx.Done():
				return
			case value, ok := <-causal.subscribers["book"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("causal book channel closed"))
					return
				}

				delta, deltaOK := value.Value.(market.BookLevelsDelta)
				if !deltaOK {
					errnie.Error(fmt.Errorf("causal: invalid book payload: %T", value.Value))
					continue
				}

				raw, ok := causal.symbols.Load(delta.Symbol)

				if ok {
					state := raw.(*CausalSymbol)
					state.FeedBook(delta)

					if _, seen := causal.requested.Load(delta.Symbol); !seen &&
						len(delta.Bids) > 0 && len(delta.Asks) > 0 {
						causal.requested.Store(delta.Symbol, struct{}{})
						causal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{delta.Symbol}})
						causal.publishPulse()
					}
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-causal.ctx.Done():
				return
			case value, ok := <-causal.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("causal feedback channel closed"))
					return
				}

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("causal: invalid feedback payload: %T", value.Value))
					continue
				}

				causal.Feedback(fb)
				causal.publishPulse()
			}
		}
	})

	wg.Wait()
	return causal.ctx.Err()
}

func (causal *Causal) requestedCount() int {
	count := 0

	causal.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (causal *Causal) publishPulse() {
	scanCap := max(config.System.MaxScanSymbols/8, 1)
	requested := causal.requestedCount()

	if len(causal.pending) > 0 && requested < scanCap {
		remaining := scanCap - requested
		batch := min(min(config.System.SubscribeBatch, remaining), len(causal.pending))

		symbols := causal.pending[:batch]
		causal.pending = causal.pending[batch:]

		for _, symbol := range symbols {
			causal.requested.Store(symbol, struct{}{})
		}

		causal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	causal.publishMeasurements()
}

func (causal *Causal) publishMeasurements() {
	now := time.Now()
	contagion := causal.contagion()
	waiters := make([]chan *qpool.QValue[any], 0)

	causal.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := causal.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*CausalSymbol)
		macro := causal.macroMomentum(symbol)
		waiters = append(
			waiters,
			causal.pool.ScheduleFast(causal.ctx, func(ctx context.Context) (any, error) {
				measurement, ok := state.Measure(macro, contagion, now)

				if !ok {
					return nil, nil
				}

				return measurement, nil
			}),
		)

		return true
	})

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

		causal.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (causal *Causal) Close() error {
	causal.cancel()
	return nil
}

func (causal *Causal) Source() string { return causalSource }

func (causal *Causal) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		now := time.Now()
		contagion := causal.contagion()

		causal.symbols.Range(func(key, value any) bool {
			symbol := key.(string)

			if _, subscribed := causal.requested.Load(symbol); !subscribed {
				return true
			}

			state := value.(*CausalSymbol)
			macro := causal.macroMomentum(symbol)
			measurement, ok := state.Measure(macro, contagion, now)

			if !ok {
				return true
			}

			return yield(measurement)
		})
	}
}

func (causal *Causal) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, causalSource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := causal.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*CausalSymbol)
	state.ApplyFeedback(feedback)
}

/*
macroMomentum returns the median change_pct across every symbol *other than*
candidate. The candidate's own change is excluded because it would otherwise
appear on both sides of the structural regression — as both the outcome
(via priceVelocity) and as a regressor (via macro) — producing contemporaneous
self-correlation that the backdoor / counterfactual estimands quietly inherit.
For tiny universes where excluding the candidate would leave fewer than two
observations the function returns 0 rather than fabricating a degenerate one.
*/
func (causal *Causal) macroMomentum(candidate string) float64 {
	changes := make([]float64, 0)

	causal.symbols.Range(func(key, value any) bool {
		symbol, _ := key.(string)

		if symbol == candidate {
			return true
		}

		state := value.(*CausalSymbol)

		if changePct := state.ChangePct(); changePct != 0 {
			changes = append(changes, changePct)
		}

		return true
	})

	if len(changes) < 2 {
		return 0
	}

	return numeric.PercentileSorted(numeric.CopySorted(changes), 0.5)
}
