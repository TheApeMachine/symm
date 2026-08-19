package exhaust

import (
	"context"
	"iter"
	"math"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal tracks side-specific microstructure decay that can advise whether a long
or short position should exit. It emits numerical family scores, their fused
urgency, and the winning numerical family identifier for downstream logic.

The signal is thin: nomagique's DecaySample owns every per-symbol window
(depth/density/spread statistics, pressure extrema, imbalance prior-mean, and
maturity), and equation.Decay owns the scoring. This file only feeds frames in
and maps the output back out.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	books      websocket.BookSource
	instrument *broker.Instrument
	sample     *algorithm.DecaySample
	decay      *equation.Decay
}

const maximumDecayCategory = 4

/*
NewSignal constructs the market-wide exhaustion observer. Position inventory is
deliberately absent: the signal measures both hypothetical exit sides, leaving
the consumer to select the side matching its position.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
	instrument *broker.Instrument,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:        ctx,
		cancel:     cancel,
		books:      books,
		instrument: instrument,
		sample:     algorithm.NewDecaySample(),
		decay:      equation.NewDecay(),
	}
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceExhaustion)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceExhaustion
}

func (signal *Signal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) iter.Seq[*types.Measurement] {
	return signal.measure(symbol)
}

func (signal *Signal) measure(
	symbol *types.Symbol,
) iter.Seq[*types.Measurement] {
	return func(yield func(*types.Measurement) bool) {
		if signal == nil || symbol == nil || signal.books == nil {
			return
		}

		tickSize := 1.0

		if signal.instrument != nil {
			pair := signal.instrument.Pair(symbol.Symbol)
			tickSize = pair.TickSize.Float64()
		}

		signal.books.Book(symbol.Symbol, func(managed *spotbook.Book) {
			if managed != nil {
				if !yield(signal.bookReading(managed, tickSize)) {
					return
				}
			}
		})

		for trade := range symbol.MarketTrades(types.SourceExhaustion) {
			if !validTrade(trade) {
				continue
			}

			if !yield(signal.tradeReading(trade)) {
				return
			}
		}
	}
}

/*
bookReading folds the current managed book into the per-symbol decay window
and returns one measurement. An immature window is still an honest zero
reading — the signal always emits when it observes a frame.
*/
func (signal *Signal) bookReading(
	managed *spotbook.Book,
	tickSize float64,
) *types.Measurement {
	if signal.sample == nil || signal.decay == nil {
		return immatureReading(managed.Name, managedBookObservedAt(managed))
	}

	observedAt := managedBookObservedAt(managed)
	bestBid, bestAsk := managed.BestBid(), managed.BestAsk()

	input := flow.BookInput{
		Symbol:   managed.Name,
		TickSize: tickSize,
		Bids:     bookSide(managed, false, tickSize),
		Asks:     bookSide(managed, true, tickSize),
	}

	decayInput, ready, maturity, err := signal.sample.MeasureBook(input)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure book",
			err,
		))

		return immatureReading(managed.Name, observedAt)
	}

	if !ready || bestBid == nil || bestAsk == nil {
		return immatureReading(managed.Name, observedAt)
	}

	output, err := signal.decay.Measure(decayInput)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure decay",
			err,
		))

		return immatureReading(managed.Name, observedAt)
	}

	measurement := signal.frame(managed.Name, observedAt, output, maturity)
	measurement.PutMetric(types.MetricBestPrice, types.SideBuy, types.MetricSample{
		Raw:  bestBid.Price.Float64(),
		Unit: types.UnitQuoteCurrency,
	})
	measurement.PutMetric(types.MetricBestPrice, types.SideSell, types.MetricSample{
		Raw:  bestAsk.Price.Float64(),
		Unit: types.UnitQuoteCurrency,
	})
	measurement.PutMetric(types.MetricTouchQuantity, types.SideBuy, types.MetricSample{
		Raw:  bestBid.Quantity.Float64(),
		Unit: types.UnitBaseCurrency,
	})
	measurement.PutMetric(types.MetricTouchQuantity, types.SideSell, types.MetricSample{
		Raw:  bestAsk.Quantity.Float64(),
		Unit: types.UnitBaseCurrency,
	})
	measurement.PutMetric(types.MetricMidpoint, types.SideNone, types.MetricSample{
		Raw:  (bestBid.Price.Float64() + bestAsk.Price.Float64()) / 2,
		Unit: types.UnitQuoteCurrency,
	})

	return measurement
}

/*
tradeReading applies one trade event to the shared decay window and returns
one measurement. A trade before the window's book established its depth ratios
still yields an honest zero reading.
*/
func (signal *Signal) tradeReading(row kraken.TradeData) *types.Measurement {
	if signal.sample == nil || signal.decay == nil {
		return immatureReading(row.Symbol, row.Timestamp)
	}

	decayInput, ready, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     flow.TradeSide(row.Side),
		At:       row.Timestamp,
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure trade",
			err,
		))

		return immatureReading(row.Symbol, row.Timestamp)
	}

	if !ready {
		return immatureReading(row.Symbol, row.Timestamp)
	}

	output, err := signal.decay.Measure(decayInput)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure decay",
			err,
		))

		return immatureReading(row.Symbol, row.Timestamp)
	}

	measurement := signal.frame(row.Symbol, row.Timestamp, output, maturity)
	measurement.PutMetric(types.MetricTradePrice, types.SideNone, types.MetricSample{
		Raw:  row.Price.Float64(),
		Unit: types.UnitQuoteCurrency,
	})
	measurement.PutMetric(types.MetricTradeQuantity, types.SideNone, types.MetricSample{
		Raw:  row.Qty,
		Unit: types.UnitBaseCurrency,
	})

	return measurement
}

/*
immatureReading is the honest zero reading for a frame the decay window cannot
score yet. It carries the symbol and event time so the pass is observable.
*/
func immatureReading(symbol string, at time.Time) *types.Measurement {
	return &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceExhaustion,
		Symbol:   symbol,
		At:       at,
		Maturity: 0,
		Metrics:  map[string]types.MetricSample{},
	}
}

/*
bookSide flattens one managed book side into the nomagique book-level shape.
*/
func bookSide(managed *spotbook.Book, ask bool, tickSize float64) []flow.BookLevel {
	var levels []flow.BookLevel

	if ask {
		for level := managed.Asks.Low; level != nil; level = level.Higher {
			levels = append(levels, flow.BookLevel{
				Price:    level.Price.Float64(),
				Quantity: level.Quantity.Float64(),
				Ticks:    int64(level.Price.Float64()/tickSize + 0.5),
			})
		}

		return levels
	}

	for level := managed.Bids.High; level != nil; level = level.Lower {
		levels = append(levels, flow.BookLevel{
			Price:    level.Price.Float64(),
			Quantity: level.Quantity.Float64(),
			Ticks:    int64(level.Price.Float64()/tickSize + 0.5),
		})
	}

	return levels
}

func managedBookObservedAt(managed *spotbook.Book) time.Time {
	if managed == nil {
		return time.Time{}
	}

	bestBid := managed.BestBid()
	bestAsk := managed.BestAsk()
	var observedAt time.Time

	if bestBid != nil {
		observedAt = bestBid.Timestamp
	}

	if bestAsk != nil && bestAsk.Timestamp.After(observedAt) {
		observedAt = bestAsk.Timestamp
	}

	return observedAt
}

func validTrade(row kraken.TradeData) bool {
	return row.Symbol != "" && !row.Timestamp.IsZero() && row.Price.Sign() > 0 &&
		row.Qty > 0 && (row.Side == "buy" || row.Side == "sell")
}

/*
frame converts a decay calculator output into the shared Measurement shape, so
both the book-driven and trade-driven observation paths emit the same metric
set for a symbol.
*/
func (signal *Signal) frame(
	symbol string,
	at time.Time,
	output equation.DecayOutput,
	maturity float64,
) *types.Measurement {
	metrics, valid := normalizedDecayMetrics(output)

	if !valid {
		panic("exhaust: decay output outside its defined metric domain")
	}

	measurement := &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceExhaustion,
		Symbol:   symbol,
		At:       at,
		Maturity: maturity,
		Metrics:  metrics,
	}
	separation, separationReady := types.MeasurementHypothesisSeparation(
		types.SourceExhaustion,
		measurement.Metrics,
	)

	if !separationReady {
		panic("exhaust: competing metric groups are not measurable")
	}

	measurement.PutMetric(types.MetricHypothesisSeparation, types.SideNone, types.MetricSample{
		Raw:        separation,
		Normalized: &separation,
		Unit:       types.UnitDimensionless,
	})

	return measurement
}

/*
normalizedDecayMetrics accepts the probability margins emitted by Decay in
[0, 1]. Category is a nominal family identifier, so it deliberately has no
scalar normalization; treating its integer code as magnitude would invent an
ordering between mechanical, fragile, thermal, and reversal exhaustion.
*/
func normalizedDecayMetrics(
	output equation.DecayOutput,
) (map[string]types.MetricSample, bool) {
	type reading struct {
		metric types.MetricType
		side   types.MeasurementSide
		raw    float64
	}

	readings := []reading{
		{types.MetricMechanical, types.SideBuy, output.Long.Mechanical},
		{types.MetricThermal, types.SideBuy, output.Long.Thermal},
		{types.MetricFragile, types.SideBuy, output.Long.Fragile},
		{types.MetricReversal, types.SideBuy, output.Long.Reversal},
		{types.MetricUrgency, types.SideBuy, output.Long.Urgency},
		{types.MetricStrength, types.SideBuy, output.Long.Strength},
		{types.MetricValue, types.SideBuy, output.Long.Value},
		{types.MetricCategory, types.SideBuy, output.Long.Category},
		{types.MetricMechanical, types.SideSell, output.Short.Mechanical},
		{types.MetricThermal, types.SideSell, output.Short.Thermal},
		{types.MetricFragile, types.SideSell, output.Short.Fragile},
		{types.MetricReversal, types.SideSell, output.Short.Reversal},
		{types.MetricUrgency, types.SideSell, output.Short.Urgency},
		{types.MetricStrength, types.SideSell, output.Short.Strength},
		{types.MetricValue, types.SideSell, output.Short.Value},
		{types.MetricCategory, types.SideSell, output.Short.Category},
	}
	metrics := make(map[string]types.MetricSample, len(readings))
	valid := true

	for _, item := range readings {
		sample := types.MetricSample{Raw: item.raw, Unit: types.UnitDimensionless}

		if item.metric == types.MetricCategory {
			if !validDecayCategory(item.raw) {
				valid = false
			}

			metrics[types.MetricKey(item.metric, item.side)] = sample
			continue
		}

		sample.Normalized = normalizedDecayScore(item.raw)

		if sample.Normalized == nil {
			valid = false
		}

		metrics[types.MetricKey(item.metric, item.side)] = sample
	}

	return metrics, valid
}

func normalizedDecayScore(raw float64) *float64 {
	if raw < 0 || raw > 1 {
		return nil
	}

	value := raw

	return &value
}

func validDecayCategory(raw float64) bool {
	return raw >= 0 && raw <= maximumDecayCategory &&
		raw == math.Trunc(raw)
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
