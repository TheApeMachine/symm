package toxicity

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
	"github.com/theapemachine/symm/kraken/level3"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

const toxicitySource = "bookflow"

// toxPulseInterval is the cadence at which the cancel-to-fill asymmetry is
// re-measured and published.
const toxPulseInterval = 2 * time.Second

// l3FreshWindow is how recently a symbol must have produced an L3 order event
// for the public L2 book to be considered redundant for it. Inside the window
// L3 is authoritative and the L2 fallback is skipped to avoid double-counting;
// outside it (or with no L3 client at all) the symbol rides the L2 proxy.
const l3FreshWindow = 30 * time.Second

/*
Toxicity is the executed-flow book-quality signal. It feeds the shared Tracker
order-by-order from the authenticated L3 feed when available, and from the
public L2 book otherwise, joining both against the public trade tape to split
liquidity removals into fills vs cancels. It publishes a directional "bookflow"
measurement from the cancel-to-fill asymmetry, and (via the package-level
IsToxic) lets the weighted-book reader exclude toxic near-touch walls.
*/
type Toxicity struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	tracker     *Tracker
	pairs       sync.Map // symbol -> asset.Pair (quote-currency symbols)
	l3Seen      sync.Map // symbol -> time.Time of last L3 order event
}

func NewToxicity(ctx context.Context, pool *qpool.Q) *Toxicity {
	ctx, cancel := context.WithCancel(ctx)

	tox := &Toxicity{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		tracker:     Default(),
	}

	for _, channel := range []string{"symbols", "tick", "trade", "book", "level3", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		tox.subscribers[channel] = group.Subscribe("toxicity:"+channel, 128)
	}

	tox.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return tox
}

func (tox *Toxicity) Start() error {
	return nil
}

func (tox *Toxicity) State() engine.State {
	return engine.READY
}

func (tox *Toxicity) Tick() error {
	errnie.Info("starting toxicity tick")

	var wg sync.WaitGroup

	wg.Go(func() { tox.consumeSymbols() })
	wg.Go(func() { tox.consumeTrades() })
	wg.Go(func() { tox.consumeTicks() })
	wg.Go(func() { tox.consumeBook() })
	wg.Go(func() { tox.consumeLevel3() })
	wg.Go(func() { tox.consumeFeedback() })
	wg.Go(func() { tox.runPulse() })

	wg.Wait()

	return tox.ctx.Err()
}

func (tox *Toxicity) consumeSymbols() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case value, ok := <-tox.subscribers["symbols"].Incoming:
			if !ok {
				errnie.Error(fmt.Errorf("toxicity symbols channel closed"))
				return
			}

			pairs, pairsOK := value.Value.(map[string]*asset.Pair)

			if !pairsOK {
				errnie.Error(fmt.Errorf("toxicity: invalid symbols payload: %T", value.Value))
				continue
			}

			for symbol, pair := range pairs {
				if pair == nil || pair.Quote != config.System.QuoteCurrency {
					continue
				}

				tox.pairs.Store(symbol, *pair)
			}
		}
	}
}

func (tox *Toxicity) consumeTrades() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case value, ok := <-tox.subscribers["trade"].Incoming:
			if !ok {
				errnie.Error(fmt.Errorf("toxicity trade channel closed"))
				return
			}

			tick := value.Value.(trade.Data)
			pair, ok := tox.pairFor(tick.Symbol)

			if !ok {
				continue
			}

			tox.tracker.ObserveTrade(tick.Symbol, pair, tick.Price, tick.Qty, tick.Timestamp)
		}
	}
}

func (tox *Toxicity) consumeTicks() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case value, ok := <-tox.subscribers["tick"].Incoming:
			if !ok {
				errnie.Error(fmt.Errorf("toxicity tick channel closed"))
				return
			}

			row := value.Value.(market.TickerRow)
			pair, ok := tox.pairFor(row.Symbol)

			if !ok {
				continue
			}

			tox.tracker.ObserveMid(row.Symbol, pair, midOf(row))
		}
	}
}

func (tox *Toxicity) consumeBook() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case value, ok := <-tox.subscribers["book"].Incoming:
			if !ok {
				errnie.Error(fmt.Errorf("toxicity book channel closed"))
				return
			}

			delta := value.Value.(market.BookLevelsDelta)
			pair, ok := tox.pairFor(delta.Symbol)

			if !ok {
				continue
			}

			// L2 fallback only: inside the L3 freshness window the
			// authenticated per-order feed is authoritative for this symbol,
			// so the aggregated public book would double-count. Process the
			// public book only for symbols L3 is not actively covering.
			if tox.l3Active(delta.Symbol, time.Now()) {
				continue
			}

			now := time.Now()

			for _, level := range delta.Bids {
				tox.tracker.ApplyBookLevel(delta.Symbol, pair, level3.SideBid, level.Price, level.Volume, now)
			}

			for _, level := range delta.Asks {
				tox.tracker.ApplyBookLevel(delta.Symbol, pair, level3.SideAsk, level.Price, level.Volume, now)
			}
		}
	}
}

func (tox *Toxicity) consumeLevel3() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case value, ok := <-tox.subscribers["level3"].Incoming:
			if !ok {
				errnie.Error(fmt.Errorf("toxicity level3 channel closed"))
				return
			}

			orders, ok := value.Value.([]level3.Order)

			if !ok {
				continue
			}

			now := time.Now()

			for _, ord := range orders {
				pair, _ := tox.pairFor(ord.Symbol)

				if pair.Wsname == "" {
					pair = asset.Pair{Wsname: ord.Symbol}
				}

				tox.l3Seen.Store(ord.Symbol, now)
				tox.tracker.ApplyOrder(
					ord.Symbol, pair, ord.Event, ord.OrderID,
					ord.Side, ord.Price, ord.Qty, ord.Ts, now,
				)
			}
		}
	}
}

func (tox *Toxicity) consumeFeedback() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case _, ok := <-tox.subscribers["feedback"].Incoming:
			if !ok {
				return
			}
			// bookflow carries no learned per-symbol state; feedback is drained
			// only to keep the subscriber from backing up.
		}
	}
}

func (tox *Toxicity) runPulse() {
	ticker := time.NewTicker(toxPulseInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tox.ctx.Done():
			return
		case <-ticker.C:
			tox.publishMeasurements()
		}
	}
}

func (tox *Toxicity) publishMeasurements() {
	now := time.Now()

	tox.pairs.Range(func(key, _ any) bool {
		symbol := key.(string)

		if measurement, ok := tox.tracker.Measure(symbol, now); ok {
			tox.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}

		return true
	})
}

func (tox *Toxicity) pairFor(symbol string) (asset.Pair, bool) {
	if raw, ok := tox.pairs.Load(symbol); ok {
		return raw.(asset.Pair), true
	}

	return asset.Pair{}, false
}

func (tox *Toxicity) l3Active(symbol string, now time.Time) bool {
	raw, ok := tox.l3Seen.Load(symbol)

	if !ok {
		return false
	}

	return now.Sub(raw.(time.Time)) < l3FreshWindow
}

func (tox *Toxicity) Close() error {
	tox.cancel()

	return nil
}

func (tox *Toxicity) Source() string {
	return toxicitySource
}

func (tox *Toxicity) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		now := time.Now()

		tox.pairs.Range(func(key, _ any) bool {
			measurement, ok := tox.tracker.Measure(key.(string), now)

			if !ok {
				return true
			}

			return yield(measurement)
		})
	}
}

// Feedback is a no-op: bookflow confidence is derived from current executed
// flow, not from settled predictions. Satisfies engine.Signal.
func (tox *Toxicity) Feedback(feedback engine.PredictionFeedback) {
	_ = feedback
}

func midOf(row market.TickerRow) float64 {
	if row.Bid > 0 && row.Ask > 0 {
		return (row.Bid + row.Ask) / 2
	}

	return row.Last
}
