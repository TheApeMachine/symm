package fluid

import (
	"context"
	"iter"
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
	scanner   *engine.Scanner
	book      *kbook.Book
	trades    *trades.Trades
	ticker    *kticker.Ticker
	track     *TrackStore
	pairs     map[string]asset.Pair
	symbols   []string
	fieldSink FieldSink
}

var _ engine.Signal = (*Fluid)(nil)

/*
NewFluid wires live Kraken websocket observers into the engine signal.
*/
func NewFluid(
	ctx context.Context,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	interval time.Duration,
) (*Fluid, error) {
	fluid := &Fluid{
		scanner: engine.NewScanner(ctx, interval),
		book:    book,
		trades:  tradesObserver,
		ticker:  tickerObserver,
		track:   NewTrackStore(),
		pairs:   pairs,
		symbols: symbols,
	}

	return fluid, errnie.Require(map[string]any{
		"scanner": fluid.scanner,
		"book":    book,
		"trades":  tradesObserver,
		"ticker":  tickerObserver,
		"track":   fluid.track,
		"pairs":   pairs,
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
	fluid.scanner.Run(fluid.scan)
}

/*
Measure yields queued measurements for the trader.
*/
func (fluid *Fluid) Measure(ctx context.Context) iter.Seq[engine.Measurement] {
	return fluid.scanner.Measure(ctx)
}

/*
Close stops field sampling.
*/
func (fluid *Fluid) Close() error {
	return fluid.scanner.Close()
}

func (fluid *Fluid) scan(now time.Time) {
	for _, symbol := range fluid.symbols {
		price, priceOK := fluid.ticker.Last(symbol)
		volumeBase, volumeOK := fluid.ticker.VolumeBase(symbol)

		if priceOK && volumeOK && price > 0 {
			fluid.track.ApplyTicker(symbol, price, volumeBase)
		}

		confidence, reason := fluid.evaluate(symbol, now)

		if confidence <= 0 {
			continue
		}

		pair, ok := fluid.pairs[symbol]

		if !ok {
			continue
		}

		fluid.scanner.Enqueue(engine.Measurement{
			Type:       engine.Flow,
			Source:     "fluid",
			Regime:     "flow",
			Reason:     reason,
			Pairs:      []asset.Pair{pair},
			Confidence: confidence,
			Timeframe:  engine.Timeframe{Start: now.UnixNano(), End: now.UnixNano()},
		})
	}

	if fluid.fieldSink != nil {
		fluid.fieldSink(fluid.FieldSnapshot())
	}
}

func (fluid *Fluid) evaluate(symbol string, now time.Time) (float64, string) {
	if !fluid.track.PassesLiquidity(symbol) {
		return 0, ""
	}

	density, densityOK := fluid.book.Density(symbol)
	spreadBPS, spreadOK := fluid.book.SpreadBPS(symbol)
	price, priceOK := fluid.ticker.Last(symbol)
	batchVolume, batchOK := fluid.trades.BatchVolume(symbol)
	buyPressure, pressureOK := fluid.trades.BuyPressure(symbol)

	if !densityOK || !spreadOK || !priceOK || !batchOK || !pressureOK {
		return 0, ""
	}

	if density <= 0 || spreadBPS <= 0 || price <= 0 || batchVolume <= 0 {
		return 0, ""
	}

	flow := batchVolume

	if buyPressure > 0 {
		flow = batchVolume * (buyPressure + 1) / 2
	}

	return fluid.track.Sample(symbol, density, price, spreadBPS, flow, buyPressure, now)
}
