package pumpdump

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Signal owns the ignition perspective's metrics: the geometry and dynamics of
the order-book ladder, plus the tape confirmation on the volume clock.

The ladder is the precursor instrument. Every pass conditions the
authoritative managed book into resting depth per side, touch spread, and the
pass's signed depth change, and folds them into time-elastic baselines whose
adaptation is driven by the symbol's own event-time gaps. Prints only confirm:
each executed trade advances the volume-clocked ignition families and raises
maturity. The signal never classifies and never judges honesty — spoof
detection belongs to the toxicity perspective, and the pump or dump verdict is
downstream logic combining both.

It always produces: a dirty pass yields one measurement carrying whatever the
ladder and the tape currently support, with maturity reporting the weakest
support count.
*/
type Signal struct {
	ctx       context.Context
	cancel    context.CancelFunc
	books     websocket.BookSource
	quotes    *data.Series[[2]float64]
	ladders   *nomagique.KeyedStreams[string]
	ignitions *nomagique.KeyedStreams[string]
	anchors   *nomagique.KeyedStreams[string]
	spreads   *nomagique.KeyedStreams[string]
	depths    *sync.Map
	halflife  float64
	capacity  float64

	fastHalflife       float64
	slowHalflife       float64
	dispersionHalflife float64
}

/*
NewSignal creates ignition state with its own quote history.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
) *Signal {
	return NewSignalWithQuotes(
		ctx,
		books,
		data.MustNewSeries[[2]float64](system.Cfg.PumpDump.Capacity),
	)
}

/*
NewSignalWithQuotes creates ignition state sharing the owning tape shard's
causal quote history.
*/
func NewSignalWithQuotes(
	ctx context.Context,
	books websocket.BookSource,
	quotes *data.Series[[2]float64],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:                ctx,
		cancel:             cancel,
		books:              books,
		quotes:             quotes,
		ladders:            nomagique.NewKeyedStreams[string](algo.Ladder, nil),
		ignitions:          nomagique.NewKeyedStreams[string](algo.Ignition, nil),
		anchors:            nomagique.NewKeyedStreams[string](detachMachine(), nil),
		spreads:            nomagique.NewKeyedStreams[string](detachMachine(), nil),
		depths:             &sync.Map{},
		halflife:           system.Cfg.PumpDump.Halflife,
		capacity:           float64(system.Cfg.PumpDump.Capacity),
		fastHalflife:       system.Cfg.PumpDump.FastHalflife,
		slowHalflife:       system.Cfg.PumpDump.SlowHalflife,
		dispersionHalflife: system.Cfg.PumpDump.DispersionHalflife,
	}

	return signal
}

/*
detachMachine is the adaptive anchor: retention first, the efficiency-driven
baseline observing the retained window, and the residual dispersion observing
the baseline.
*/
func detachMachine() nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Window,
		nomagique.Fork(statistic.Baseline, statistic.ZScore),
	)
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourcePumpDump)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourcePumpDump
}

/*
Measure produces the ignition perspective's measurements for one dirty symbol.
*/
func (signal *Signal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) iter.Seq[*types.Measurement] {
	return func(yield func(*types.Measurement) bool) {
		signal.ingestQuotes(symbol)

		at := signal.drainLevel3(symbol)

		snapshot := signal.bookSnapshot(symbol)

		if snapshot.observedAt.After(at) {
			at = snapshot.observedAt
		}

		metrics := map[string]types.MetricSample{}
		maturity := 0.0

		// The ladder clock is event time only. A pass without any observed
		// event time carries no ladder observation; fabricating one from the
		// local wall clock would poison the ladder's monotone clock domain.
		if snapshot.ready && !at.IsZero() {
			signal.measureLadder(symbol.Symbol, snapshot, at, metrics, &maturity)
		}

		if snapshot.ready && !at.IsZero() && snapshot.bid > 0 && snapshot.ask > snapshot.bid {
			signal.measureDetach(symbol.Symbol, snapshot, at, metrics, &maturity)
		}

		signal.measureTape(symbol, snapshot, metrics, &maturity)

		if snapshot.ready {
			snapshot.putMetrics(metrics)
		}

		separation, separationReady := types.MeasurementHypothesisSeparation(
			types.SourcePumpDump, metrics,
		)

		if separationReady {
			metrics[types.MetricKey(types.MetricHypothesisSeparation, types.SideNone)] = types.MetricSample{
				Raw:        separation,
				Normalized: &separation,
				Unit:       types.UnitDimensionless,
			}
		}

		yield(&types.Measurement{
			ID:       uuid.NewString(),
			Source:   types.SourcePumpDump,
			Symbol:   symbol.Symbol,
			Tick:     symbol.Tick,
			At:       at,
			Maturity: maturity,
			Metrics:  metrics,
		})
	}
}

/*
measureLadder folds one pass of book aggregates into the symbol's ladder
stream and maps the ladder output onto measurement metrics.
*/
func (signal *Signal) measureLadder(
	symbolName string,
	snapshot bookSnapshot,
	at time.Time,
	metrics map[string]types.MetricSample,
	maturity *float64,
) {
	previous, _ := signal.depths.LoadOrStore(symbolName, snapshot.depths())
	prior := previous.(ladderDepths)
	signal.depths.Store(symbolName, snapshot.depths())

	input := nomagique.Frame{}
	input.Put(algo.SymbolLadderHalflife, signal.halflife)
	input.Put(algo.SymbolLadderBidDepth, snapshot.bidDepth)
	input.Put(algo.SymbolLadderAskDepth, snapshot.askDepth)
	input.Put(algo.SymbolLadderSpread, snapshot.spread)
	input.Put(algo.SymbolLadderBidDelta, snapshot.bidDepth-prior.bid)
	input.Put(algo.SymbolLadderAskDelta, snapshot.askDepth-prior.ask)
	input.Put(algo.SymbolUnixSec, float64(at.Unix()))
	input.Put(algo.SymbolUnixNsec, float64(at.Nanosecond()))

	output, err := signal.ladders.Step(symbolName, input)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"pumpdump: ladder failed for "+symbolName,
			err,
		))

		return
	}

	ladder := map[nomagique.Symbol]types.MetricType{
		algo.SymbolLadderBidDepth:       types.MetricLadderBidDepth,
		algo.SymbolLadderAskDepth:       types.MetricLadderAskDepth,
		algo.SymbolLadderImbalance:      types.MetricLadderImbalance,
		algo.SymbolLadderBidDepletion:   types.MetricLadderBidDepletion,
		algo.SymbolLadderAskDepletion:   types.MetricLadderAskDepletion,
		algo.SymbolLadderBidReplenish:   types.MetricLadderBidReplenish,
		algo.SymbolLadderAskReplenish:   types.MetricLadderAskReplenish,
		algo.SymbolLadderSpreadBaseline: types.MetricLadderSpreadBaseline,
		algo.SymbolLadderCompression:    types.MetricCompression,
	}

	for source, metric := range ladder {
		value, found := output.Get(source)

		if !found {
			continue
		}

		if metric == types.MetricCompression {
			normalized := value
			metrics[types.MetricKey(metric, types.SideNone)] = types.MetricSample{
				Raw:        value,
				Normalized: &normalized,
				Unit:       types.UnitDimensionless,
			}

			continue
		}

		metrics[types.MetricKey(metric, types.SideNone)] = types.MetricSample{
			Raw:  value,
			Unit: unitForLadderMetric(metric),
		}
	}

	if value, found := output.Get(algo.SymbolLadderMaturity); found && *maturity < value {
		*maturity = value
	}
}

/*
measureDetach feeds this pass's midpoint and touch spread through the
adaptive anchor machines and maps the readings onto measurement metrics: the
value's deviation and lift from its own anchor, and the spread's deviation
and lift — a tightening spread reads as a negative lift.
*/
func (signal *Signal) measureDetach(
	symbolName string,
	snapshot bookSnapshot,
	at time.Time,
	metrics map[string]types.MetricSample,
	maturity *float64,
) {
	midpoint := (snapshot.bid + snapshot.ask) / 2

	anchorOutput, err := signal.anchors.Step(
		symbolName, signal.detachInput(midpoint, at),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"pumpdump: anchor machine failed for "+symbolName,
			err,
		))

		return
	}

	spreadOutput, err := signal.spreads.Step(
		symbolName, signal.detachInput(snapshot.spread, at),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"pumpdump: spread machine failed for "+symbolName,
			err,
		))

		return
	}

	detach, _ := anchorOutput.Get(statistic.SymbolZScore)
	metrics[types.MetricKey(types.MetricAnchorDetach, types.SideNone)] = types.MetricSample{
		Raw: detach, Unit: types.UnitDimensionless,
	}

	anchorBaseline, hasAnchorBaseline := anchorOutput.Get(statistic.SymbolBaselineValue)

	if hasAnchorBaseline {
		lift := signal.liftOver(midpoint, anchorBaseline)

		metrics[types.MetricKey(types.MetricAnchorLift, types.SideNone)] = types.MetricSample{
			Raw: lift, Unit: types.UnitDimensionless,
		}
	}

	deviation, _ := spreadOutput.Get(statistic.SymbolZScore)
	metrics[types.MetricKey(types.MetricSpreadDeviation, types.SideNone)] = types.MetricSample{
		Raw: deviation, Unit: types.UnitDimensionless,
	}

	spreadBaseline, hasSpreadBaseline := spreadOutput.Get(statistic.SymbolBaselineValue)

	if hasSpreadBaseline {
		tightening := signal.liftOver(snapshot.spread, spreadBaseline)

		metrics[types.MetricKey(types.MetricSpreadTightening, types.SideNone)] = types.MetricSample{
			Raw: tightening, Unit: types.UnitDimensionless,
		}
	}

	if count, found := anchorOutput.Get(nomagique.SampleCount); found {
		window := count / (count + 1)

		if *maturity < window {
			*maturity = window
		}
	}
}

/*
detachInput builds one observation for the anchor machines: the value on the
pass's event time with the composed adaptation horizons.
*/
func (signal *Signal) detachInput(value float64, at time.Time) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(nomagique.SampleValue, value)
	input.Put(temporal.SymbolCapacity, signal.capacity)
	input.Put(statistic.SymbolUnixSec, float64(at.Unix()))
	input.Put(statistic.SymbolUnixNsec, float64(at.Nanosecond()))
	input.Put(statistic.SymbolBaselineFastHalflife, signal.fastHalflife)
	input.Put(statistic.SymbolBaselineSlowHalflife, signal.slowHalflife)
	input.Put(statistic.SymbolDispersionHalflife, signal.dispersionHalflife)

	return input
}

/*
liftOver answers the lift equation for one value over one baseline.
*/
func (signal *Signal) liftOver(value float64, baseline float64) float64 {
	input := nomagique.Frame{}
	input.Put(nomagique.SampleValue, value)
	input.Put(statistic.SymbolBaseline, baseline)

	_, output, err := statistic.Lift(nomagique.Frame{}, input)

	if err != nil {
		panic(err)
	}

	lift, _ := output.Get(statistic.SymbolResult)

	return lift
}

/*
measureTape advances the volume-clocked ignition families with this pass's
executed trades and maps the latest tape output onto measurement metrics: the
relative volume against the tape's own median baseline, per-side price
precursors, and per-side exhaustion. Prints confirm; they never classify.
The book's own touch answers any trade the quote history cannot, so an
executed print is never dropped for lack of a ticker.
*/
func (signal *Signal) measureTape(
	symbol *types.Symbol,
	snapshot bookSnapshot,
	metrics map[string]types.MetricSample,
	maturity *float64,
) {
	var latest nomagique.Frame
	haveOutput := false

	for trade := range symbol.MarketTrades(types.SourcePumpDump) {
		bid, ask, found := signal.quoteFor(trade, snapshot)

		if !found {
			continue
		}

		input := nomagique.Frame{}
		input.Put(algo.SymbolCapacity, signal.capacity)
		input.Put(algo.SymbolVolume, trade.Qty)
		input.Put(algo.SymbolLast, trade.Price.Float64())
		input.Put(algo.SymbolBid, bid)
		input.Put(algo.SymbolAsk, ask)
		input.Put(algo.SymbolUnixSec, float64(trade.Timestamp.Unix()))
		input.Put(algo.SymbolUnixNsec, float64(trade.Timestamp.Nanosecond()))

		output, err := signal.ignitions.Step(symbol.Symbol, input)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"pumpdump: ignition failed for "+symbol.Symbol,
				err,
			))

			continue
		}

		latest = output
		haveOutput = true
		metrics[types.MetricKey(types.MetricTradePrice, types.SideNone)] = types.MetricSample{
			Raw: trade.Price.Float64(), Unit: types.UnitQuoteCurrency,
		}
		metrics[types.MetricKey(types.MetricTradeQuantity, types.SideNone)] = types.MetricSample{
			Raw: trade.Qty, Unit: types.UnitBaseCurrency,
		}
	}

	if !haveOutput {
		return
	}

	ignition := []struct {
		symbol nomagique.Symbol
		metric types.MetricType
		side   types.MeasurementSide
	}{
		{algo.SymbolRVOL, types.MetricRVOL, types.SideNone},
		{algo.SymbolAlphaPrecursor, types.MetricPrecursor, types.SideBuy},
		{algo.SymbolAlphaExhaustion, types.MetricExhaustion, types.SideBuy},
		{algo.SymbolBetaPrecursor, types.MetricPrecursor, types.SideSell},
		{algo.SymbolBetaExhaustion, types.MetricExhaustion, types.SideSell},
	}

	for _, item := range ignition {
		value, found := latest.Get(item.symbol)

		if !found {
			continue
		}

		sample := types.MetricSample{
			Raw:  value,
			Unit: types.UnitDimensionless,
		}

		if item.metric == types.MetricRVOL || item.metric == types.MetricPrecursor {
			if value > 0 {
				normalized := value / (1 + value)
				sample.Normalized = &normalized
			}
		} else {
			normalized := value
			sample.Normalized = &normalized
		}

		metrics[types.MetricKey(item.metric, item.side)] = sample
	}

	rate, hasRate := latest.Get(algo.SymbolIgnitionBarRate)
	rateBaseline, hasRateBaseline := latest.Get(algo.SymbolIgnitionRateBaseline)

	if hasRate && hasRateBaseline {
		rvolLift := signal.liftOver(rate, rateBaseline)

		metrics[types.MetricKey(types.MetricRVOLLift, types.SideNone)] = types.MetricSample{
			Raw:  rvolLift,
			Unit: types.UnitDimensionless,
		}
	}

	if value, found := latest.Get(algo.SymbolMaturity); found && *maturity < value {
		*maturity = value
	}
}

/*
quoteFor answers one trade with the causal quote, falling back to the book's
own touch so an executed print is never dropped for lack of a ticker.
*/
func (signal *Signal) quoteFor(
	trade kraken.TradeData,
	snapshot bookSnapshot,
) (float64, float64, bool) {
	sides, found := signal.quotes.AsOf(
		trade.Symbol,
		float64(trade.Timestamp.Unix()),
		float64(trade.Timestamp.Nanosecond()),
	)

	if found {
		return sides[0], sides[1], true
	}

	if snapshot.ready && snapshot.bid > 0 && snapshot.ask > snapshot.bid {
		return snapshot.bid, snapshot.ask, true
	}

	return 0, 0, false
}

func (signal *Signal) ingestQuotes(symbol *types.Symbol) {
	for ticker := range symbol.MarketTickers(types.SourcePumpDump) {
		if ticker.Bid == nil || ticker.Ask == nil || ticker.Timestamp.IsZero() ||
			ticker.Bid.Sign() <= 0 || ticker.Ask.Cmp(ticker.Bid) <= 0 {
			continue
		}

		signal.quotes.Observe(
			ticker.Symbol,
			float64(ticker.Timestamp.Unix()),
			float64(ticker.Timestamp.Nanosecond()),
			[2]float64{ticker.Bid.Float64(), ticker.Ask.Float64()},
		)
	}
}

/*
drainLevel3 consumes this pass's accepted order frames and returns the newest
event time they carry. The frames drive the ladder's clock, and draining
keeps the queue honest even on passes without a book.
*/
func (signal *Signal) drainLevel3(symbol *types.Symbol) time.Time {
	var latest time.Time

	for frame := range symbol.MarketLevel3(types.SourcePumpDump) {
		if frame.Timestamp.After(latest) {
			latest = frame.Timestamp
		}
	}

	return latest
}

/*
ladderDepths carries one symbol's previous pass depths so a pass can report
its signed depth change without retaining the book itself.
*/
type ladderDepths struct {
	bid float64
	ask float64
}

/*
bookSnapshot is one pass's conditioning of the authoritative managed book:
full resting depth per side within the subscribed depth, the touch, and the
book's own event-time high-water mark.
*/
type bookSnapshot struct {
	ready      bool
	bid        float64
	ask        float64
	bidDepth   float64
	askDepth   float64
	spread     float64
	observedAt time.Time
}

func (snapshot bookSnapshot) depths() ladderDepths {
	return ladderDepths{bid: snapshot.bidDepth, ask: snapshot.askDepth}
}

func (snapshot bookSnapshot) putMetrics(metrics map[string]types.MetricSample) {
	metrics[types.MetricKey(types.MetricBestPrice, types.SideBuy)] = types.MetricSample{
		Raw: snapshot.bid, Unit: types.UnitQuoteCurrency,
	}
	metrics[types.MetricKey(types.MetricBestPrice, types.SideSell)] = types.MetricSample{
		Raw: snapshot.ask, Unit: types.UnitQuoteCurrency,
	}
	metrics[types.MetricKey(types.MetricMidpoint, types.SideNone)] = types.MetricSample{
		Raw: (snapshot.bid + snapshot.ask) / 2, Unit: types.UnitQuoteCurrency,
	}
	metrics[types.MetricKey(types.MetricSpread, types.SideNone)] = types.MetricSample{
		Raw: snapshot.spread, Unit: types.UnitQuoteCurrency,
	}
}

/*
bookSnapshot conditions the managed book for one symbol. The depth band is
the venue subscription itself, so the ladder never invents its own horizon.
*/
func (signal *Signal) bookSnapshot(symbol *types.Symbol) bookSnapshot {
	snapshot := bookSnapshot{}

	if signal.books == nil {
		return snapshot
	}

	_, observedAt := symbol.BookRevision()

	signal.books.Book(symbol.Symbol, func(managed *spotbook.Book) {
		if managed == nil {
			return
		}

		for level := managed.Bids.High; level != nil; level = level.Lower {
			if level.Quantity != nil {
				snapshot.bidDepth += level.Quantity.Float64()
			}
		}

		for level := managed.Asks.Low; level != nil; level = level.Higher {
			if level.Quantity != nil {
				snapshot.askDepth += level.Quantity.Float64()
			}
		}

		if bestBid := managed.BestBid(); bestBid != nil && bestBid.Price != nil {
			snapshot.bid = bestBid.Price.Float64()
		}

		if bestAsk := managed.BestAsk(); bestAsk != nil && bestAsk.Price != nil {
			snapshot.ask = bestAsk.Price.Float64()
		}

		if snapshot.bid > 0 && snapshot.ask > snapshot.bid {
			snapshot.spread = snapshot.ask - snapshot.bid
		}

		snapshot.observedAt = observedAt
		snapshot.ready = true
	})

	return snapshot
}

func unitForLadderMetric(metric types.MetricType) types.MeasurementUnit {
	switch metric {
	case types.MetricLadderBidDepth,
		types.MetricLadderAskDepth,
		types.MetricLadderBidDepletion,
		types.MetricLadderAskDepletion,
		types.MetricLadderBidReplenish,
		types.MetricLadderAskReplenish:
		return types.UnitBaseCurrency
	default:
		return types.UnitDimensionless
	}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
