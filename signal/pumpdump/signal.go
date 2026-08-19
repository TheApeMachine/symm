package pumpdump

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
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
/*
BookSource is the narrow dependency pumpdump needs from the venue: the ability
to read the resident book for one symbol. It is an interface so tests can inject
a deterministic book (or nil, to exercise the empty-book guard) instead of a live
websocket API.
*/
type BookSource interface {
	Book(symbol string, read func(*spotbook.Book))
}

/*
Signal classifies pumping vs dumping from quote-ladder ignition and trade-tape
pullback, not from ticker aggregates. It never classifies and never judges
honesty — spoof detection belongs to the toxicity perspective, and the pump or
dump verdict is downstream logic combining both.

It always produces: a dirty pass yields one measurement carrying whatever the
ladder and the tape currently support, with maturity reporting the weakest
support count.
*/
type Signal struct {
	ctx                context.Context
	cancel             context.CancelFunc
	api                BookSource
	ladders            *nomagique.KeyedStreams[string]
	ignitions          *nomagique.KeyedStreams[string]
	anchors            *nomagique.KeyedStreams[string]
	spreads            *nomagique.KeyedStreams[string]
	depths             *sync.Map
	halflife           float64
	capacity           float64
	fastHalflife       float64
	slowHalflife       float64
	dispersionHalflife float64
}

/*
NewSignal creates ignition state with its own quote history.
*/
func NewSignal(
	ctx context.Context,
	api BookSource,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:                ctx,
		cancel:             cancel,
		api:                api,
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
		at := signal.drainLevel3(symbol)

		// Produce the measurement body from a possibly-absent book. The venue
		// book is read once per pass; a dirty pass always yields one measurement
		// (the tape contract), whether or not the book is populated.
		read := func(book *spotbook.Book) {
			snapshot := bookSnapshot{}

			if book != nil {
				if bestBid := book.BestBid(); bestBid != nil {
					snapshot.bid = bestBid.Price.Float64()
					snapshot.bidDepth = bestBid.Quantity.Float64()
				}

				if bestAsk := book.BestAsk(); bestAsk != nil {
					snapshot.ask = bestAsk.Price.Float64()
					snapshot.askDepth = bestAsk.Quantity.Float64()
				}

				if snapshot.bid > 0 && snapshot.ask > snapshot.bid {
					snapshot.ready = true
					snapshot.observedAt = managedBookObservedAt(book)
					snapshot.spread = book.Spread().Float64()
				}
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

		if signal.api == nil {
			read(nil)
			return
		}

		signal.api.Book(symbol.Symbol, read)
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

	// The book touch was captured once by the outer pass read. Re-reading it
	// here would re-enter Book.Get while that outer callback still holds the
	// manager's read lock — a guaranteed deadlock as soon as the live level3
	// writer queues for the write lock (Go blocks new readers while a writer
	// waits).
	bid, ask := snapshot.bid, snapshot.ask

	for trade := range symbol.MarketTrades(types.SourcePumpDump) {
		// An executed print is always observable, even without a live book
		// touch, so the tape metrics are written before the ignition step.
		metrics[types.MetricKey(types.MetricTradePrice, types.SideNone)] = types.MetricSample{
			Raw: trade.Price.Float64(), Unit: types.UnitQuoteCurrency,
		}
		metrics[types.MetricKey(types.MetricTradeQuantity, types.SideNone)] = types.MetricSample{
			Raw: trade.Qty, Unit: types.UnitBaseCurrency,
		}

		// Without a live book touch (deferred past the pass cut, empty, or no
		// venue source), the print has no book response and the ignition
		// cannot enrich it, so the step is skipped rather than fed a fabricated
		// bid/ask pair.
		if !snapshot.ready {
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

/*
managedBookObservedAt derives the book's event-time high-water mark from its own
levels. It is the timing anchor for the ladder snapshot.
*/
func managedBookObservedAt(managed *spotbook.Book) time.Time {
	observedAt := time.Time{}

	if managed == nil {
		return observedAt
	}

	for bid := managed.Bids.High; bid != nil; bid = bid.Lower {
		if bid.Timestamp.After(observedAt) {
			observedAt = bid.Timestamp
		}
	}

	for ask := managed.Asks.Low; ask != nil; ask = ask.Higher {
		if ask.Timestamp.After(observedAt) {
			observedAt = ask.Timestamp
		}
	}

	return observedAt
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
