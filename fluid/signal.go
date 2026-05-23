package fluid

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
Fluid models order-book liquidity as a compressible field with source-sink continuity.
*/
type Fluid struct {
	ctx       context.Context
	cancel    context.CancelFunc
	book      *kbook.Book
	trades    *trades.Trades
	ticker    *kticker.Ticker
	track     *TrackStore
	pairs     map[string]asset.Pair
	symbols   []string
	interval  time.Duration
	fieldSink FieldSink
	queue     sync.Map
	seq       atomic.Int64
}

var _ engine.Signal = (*Fluid)(nil)

/*
NewFluid wires live Kraken websocket observers into the engine signal.
*/
func NewFluid(
	ctx context.Context,
	observers []engine.Observer,
	pairs map[string]asset.Pair,
	symbols []string,
	interval time.Duration,
) (*Fluid, error) {
	ctx, cancel := context.WithCancel(ctx)

	if interval <= 0 {
		interval = 100 * time.Millisecond
	}

	book, tradesObserver, tickerObserver, err := resolveMarketObservers(observers)

	if err != nil {
		cancel()
		return nil, err
	}

	fluid := &Fluid{
		ctx:      ctx,
		cancel:   cancel,
		book:     book,
		trades:   tradesObserver,
		ticker:   tickerObserver,
		track:    NewTrackStore(),
		pairs:    pairs,
		symbols:  symbols,
		interval: interval,
	}

	return fluid, errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"book":   book,
		"trades": tradesObserver,
		"ticker": tickerObserver,
		"track":  fluid.track,
		"pairs":  pairs,
	})
}

/*
SetFieldSink wires immediate field telemetry after every scan.
*/
func (fluid *Fluid) SetFieldSink(sink FieldSink) {
	fluid.fieldSink = sink
}

/*
SampledCount returns symbols with at least one fluid sample.
*/
func (fluid *Fluid) SampledCount() int {
	return fluid.track.SampledCount()
}

/*
WarmingCount returns symbols ingesting ticker volume but not yet sampled.
*/
func (fluid *Fluid) WarmingCount() int {
	return fluid.track.WarmingCount()
}

/*
Run advances the fluid field on a fixed interval.
*/
func (fluid *Fluid) Run() {
	go func() {
		ticker := time.NewTicker(fluid.interval)
		defer ticker.Stop()

		for {
			select {
			case <-fluid.ctx.Done():
				return
			case tick := <-ticker.C:
				fluid.scan(tick)
			}
		}
	}()
}

/*
Measure yields queued measurements for the trader.
*/
func (fluid *Fluid) Measure(_ context.Context) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		fluid.queue.Range(func(key, value any) bool {
			measurement, ok := value.(engine.Measurement)

			if !ok {
				errnie.Error(fmt.Errorf("invalid measurement type: %T", value))
				fluid.queue.Delete(key)
				return true
			}

			if !yield(measurement) {
				return false
			}

			fluid.queue.Delete(key)
			return true
		})
	}
}

/*
Close stops field sampling.
*/
func (fluid *Fluid) Close() error {
	fluid.cancel()
	return nil
}

func (fluid *Fluid) scan(now time.Time) {
	for _, symbol := range fluid.symbols {
		fluid.ingest(symbol, now)
	}

	if fluid.fieldSink != nil {
		fluid.fieldSink(fluid.FieldSnapshot())
	}

	for _, symbol := range fluid.symbols {
		confidence, reason, fired := fluid.fire(symbol, now)

		if !fired {
			continue
		}

		pair, ok := fluid.pairs[symbol]

		if !ok {
			continue
		}

		fluid.queue.Store(fluid.seq.Add(1), engine.Measurement{
			Type:       engine.Flow,
			Source:     "fluid",
			Regime:     "flow",
			Reason:     reason,
			Pairs:      []asset.Pair{pair},
			Confidence: confidence,
			Timeframe:  engine.Timeframe{Start: now.UnixNano(), End: now.UnixNano()},
		})
	}
}

func (fluid *Fluid) ingest(symbol string, now time.Time) {
	price, priceOK := fluid.ticker.Last(symbol)
	volumeBase, volumeOK := fluid.ticker.VolumeBase(symbol)

	if priceOK && volumeOK && price > 0 {
		fluid.track.ApplyTicker(symbol, price, volumeBase)
	}

	if !fluid.track.PassesLiquidity(symbol) {
		return
	}

	density, densityOK := fluid.book.Density(symbol)
	spreadBPS, spreadOK := fluid.book.SpreadBPS(symbol)
	batchVolume, batchOK := fluid.trades.BatchVolume(symbol)
	buyPressure, pressureOK := fluid.trades.BuyPressure(symbol)

	if !densityOK || !spreadOK || !priceOK || !batchOK || !pressureOK {
		return
	}

	if density <= 0 || spreadBPS <= 0 || price <= 0 || batchVolume <= 0 {
		return
	}

	flow := batchVolume

	if buyPressure > 0 {
		flow = batchVolume * (buyPressure + 1) / 2
	}

	fluid.track.Sample(symbol, density, price, spreadBPS, flow, buyPressure, now)
}

func (fluid *Fluid) fire(symbol string, now time.Time) (float64, string, bool) {
	if !fluid.track.PassesLiquidity(symbol) {
		return 0, "", false
	}

	density, densityOK := fluid.book.Density(symbol)
	spreadBPS, spreadOK := fluid.book.SpreadBPS(symbol)
	price, priceOK := fluid.ticker.Last(symbol)
	batchVolume, batchOK := fluid.trades.BatchVolume(symbol)
	buyPressure, pressureOK := fluid.trades.BuyPressure(symbol)

	if !densityOK || !spreadOK || !priceOK || !batchOK || !pressureOK {
		return 0, "", false
	}

	if density <= 0 || spreadBPS <= 0 || price <= 0 || batchVolume <= 0 {
		return 0, "", false
	}

	flow := batchVolume

	if buyPressure > 0 {
		flow = batchVolume * (buyPressure + 1) / 2
	}

	return fluid.track.PeekFire(symbol, density, price, spreadBPS, flow, buyPressure, now)
}
