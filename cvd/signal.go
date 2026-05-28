package cvd

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
)

const cvdSource = "cvd"

// cvdPulseInterval is the cadence at which CVD sweeps its symbols and publishes
// accumulation/distribution measurements. The window is 15 minutes and the
// forward horizon 30, so a few-second pulse is ample and bounds the per-tick
// cost (Measure is O(window trades)).
const cvdPulseInterval = 2 * time.Second

/*
CVD is the cumulative-volume-delta signal: it reads the executed trade tape and
emits an accumulation (bullish) or distribution (bearish) reading when flow is
strongly one-sided while price is suppressed. Executed flow cannot be spoofed,
so the reading is a clean input for the forward-edge model. CVD consumes the
same subscribed-symbol trade/ticker stream as the other microstructure signals;
it does not drive subscriptions itself.
*/
type CVD struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
}

func NewCVD(ctx context.Context, pool *qpool.Q) *CVD {
	ctx, cancel := context.WithCancel(ctx)

	cvd := &CVD{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "trade", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		cvd.subscribers[channel] = group.Subscribe("cvd:"+channel, 128)
	}

	cvd.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return cvd
}

func (cvd *CVD) Start() error {
	return nil
}

func (cvd *CVD) State() engine.State {
	return engine.READY
}

func (cvd *CVD) Tick() error {
	errnie.Info("starting cvd tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-cvd.ctx.Done():
				return
			case value, ok := <-cvd.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("cvd symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)

				if !pairsOK {
					errnie.Error(fmt.Errorf("cvd: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
					if pair == nil || pair.Quote != config.System.QuoteCurrency {
						continue
					}

					if _, exists := cvd.symbols.Load(symbol); !exists {
						cvd.symbols.Store(symbol, NewCVDSymbol(*pair))
					}
				}
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-cvd.ctx.Done():
				return
			case value, ok := <-cvd.subscribers["trade"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("cvd trade channel closed"))
					return
				}

				tick := value.Value.(trade.Data)
				raw, ok := cvd.symbols.Load(tick.Symbol)

				if !ok {
					continue
				}

				// side == "buy" means the taker lifted the ask (a taker buy).
				raw.(*CVDSymbol).FeedTrade(tick.Price, tick.Qty, tick.Side == "buy", tick.Timestamp)
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-cvd.ctx.Done():
				return
			case value, ok := <-cvd.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("cvd tick channel closed"))
					return
				}

				row := value.Value.(market.TickerRow)
				raw, ok := cvd.symbols.Load(row.Symbol)

				if !ok {
					continue
				}

				raw.(*CVDSymbol).FeedQuote(row.Bid, row.Ask, row.Last)
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-cvd.ctx.Done():
				return
			case value, ok := <-cvd.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("cvd feedback channel closed"))
					return
				}

				if feedback, ok := value.Value.(engine.PredictionFeedback); ok {
					cvd.Feedback(feedback)
				}
			}
		}
	})

	wg.Go(func() {
		ticker := time.NewTicker(cvdPulseInterval)
		defer ticker.Stop()

		for {
			select {
			case <-cvd.ctx.Done():
				return
			case <-ticker.C:
				cvd.publishMeasurements()
			}
		}
	})

	wg.Wait()

	return cvd.ctx.Err()
}

func (cvd *CVD) publishMeasurements() {
	now := time.Now()

	cvd.symbols.Range(func(_, value any) bool {
		measurement, ok := value.(*CVDSymbol).Measure(now)

		if ok {
			cvd.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}

		return true
	})
}

func (cvd *CVD) Close() error {
	cvd.cancel()

	return nil
}

func (cvd *CVD) Source() string {
	return cvdSource
}

func (cvd *CVD) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		now := time.Now()
		done := false

		cvd.symbols.Range(func(_, value any) bool {
			measurement, ok := value.(*CVDSymbol).Measure(now)

			if !ok {
				return true
			}

			if !yield(measurement) {
				done = true

				return false
			}

			return true
		})

		_ = done
	}
}

// Feedback is a no-op: CVD has no learned per-symbol state to update from
// settled predictions; its confidence is derived purely from current executed
// flow. It still satisfies the engine.Signal interface.
func (cvd *CVD) Feedback(feedback engine.PredictionFeedback) {
	_ = feedback
}
