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
	quotes    *types.QuoteHistory
	ladders   *nomagique.KeyedStreams[string]
	ignitions *nomagique.KeyedStreams[string]
	depths    *sync.Map
	halflife  float64
	capacity  float64
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
		types.NewQuoteHistory(system.Cfg.PumpDump.Capacity),
	)
}

/*
NewSignalWithQuotes creates ignition state sharing the owning tape shard's
causal quote history.
*/
func NewSignalWithQuotes(
	ctx context.Context,
	books websocket.BookSource,
	quotes *types.QuoteHistory,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:       ctx,
		cancel:    cancel,
		books:     books,
		quotes:    quotes,
		ladders:   nomagique.NewKeyedStreams[string](algo.Ladder, nil),
		ignitions: nomagique.NewKeyedStreams[string](algo.Ignition, nil),
		depths:    &sync.Map{},
		halflife:  system.Cfg.PumpDump.Halflife,
		capacity:  float64(system.Cfg.PumpDump.Capacity),
	}

	return signal
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

		if at.IsZero() {
			at = time.Now().UTC()
		}

		metrics := map[string]types.MetricSample{}
		maturity := 0.0

		if snapshot.ready {
			signal.measureLadder(symbol.Symbol, snapshot, at, metrics, &maturity)
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
measureTape advances the volume-clocked ignition families with this pass's
executed trades and maps the latest ignition output onto measurement metrics.
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
		{algo.SymbolPrecursor, types.MetricPrecursor, types.SideNone},
		{algo.SymbolIgnition, types.MetricIgnition, types.SideNone},
		{algo.SymbolTrend, types.MetricTrend, types.SideNone},
		{algo.SymbolExhaustion, types.MetricExhaustion, types.SideNone},
		{algo.SymbolStrength, types.MetricStrength, types.SideNone},
		{algo.SymbolBuyPrecursor, types.MetricPrecursor, types.SideBuy},
		{algo.SymbolBuyCompression, types.MetricCompression, types.SideBuy},
		{algo.SymbolBuyIgnition, types.MetricIgnition, types.SideBuy},
		{algo.SymbolBuyTrend, types.MetricTrend, types.SideBuy},
		{algo.SymbolBuyExhaustion, types.MetricExhaustion, types.SideBuy},
		{algo.SymbolBuyStrength, types.MetricStrength, types.SideBuy},
		{algo.SymbolSellPrecursor, types.MetricPrecursor, types.SideSell},
		{algo.SymbolSellCompression, types.MetricCompression, types.SideSell},
		{algo.SymbolSellIgnition, types.MetricIgnition, types.SideSell},
		{algo.SymbolSellTrend, types.MetricTrend, types.SideSell},
		{algo.SymbolSellExhaustion, types.MetricExhaustion, types.SideSell},
		{algo.SymbolSellStrength, types.MetricStrength, types.SideSell},
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
	if ticker, found := signal.quotes.At(trade.Symbol, trade.Timestamp); found {
		return ticker.Bid.Float64(), ticker.Ask.Float64(), true
	}

	if snapshot.ready && snapshot.bid > 0 && snapshot.ask > snapshot.bid {
		return snapshot.bid, snapshot.ask, true
	}

	return 0, 0, false
}

func (signal *Signal) ingestQuotes(symbol *types.Symbol) {
	for ticker := range symbol.MarketTickers(types.SourcePumpDump) {
		signal.quotes.Observe(ticker)
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
