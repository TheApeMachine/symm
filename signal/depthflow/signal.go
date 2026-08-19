package depthflow

import (
	"context"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Categories belong in logic; this
signal emits numerical scores only.

The signal is thin: nomagique's flow.Sample owns every per-symbol window and
equation.Bookflow owns the scoring. This file only feeds frames in and maps the
output back out.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	books      websocket.BookSource
	instrument *broker.Instrument
	sample     *flow.Sample
	bookflow   *equation.Bookflow
}

/*
NewSignal creates depth-flow state shared by the causally ordered book and
trade observations in each central market cut.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
	instrument *broker.Instrument,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	sample, err := flow.NewSample(viper.GetViper().GetInt("signals.depthflow.sampleSize"))

	if err != nil {
		cancel()
		errnie.Error(errnie.Err(
			errnie.Validation,
			"depthflow: failed to create flow sample",
			err,
		))

		return nil
	}

	return &Signal{
		ctx:        ctx,
		cancel:     cancel,
		books:      books,
		instrument: instrument,
		sample:     sample,
		bookflow:   equation.NewBookflow(),
	}
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceDepthFlow)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceDepthFlow
}

func (signal *Signal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) error {
	return signal.measure(symbol)
}

func (signal *Signal) measure(symbol *types.Symbol) error {
	if signal == nil || signal.books == nil {
		return nil
	}

	if symbol == nil {
		return nil
	}

	tickSize := 1.0

	if signal.instrument != nil {
		pair := signal.instrument.Pair(symbol.Symbol)
		tickSize = pair.TickSize.Float64()
	}

	var emitErr error

	signal.books.Book(symbol.Symbol, func(managed *spotbook.Book) {
		if managed == nil || emitErr != nil {
			return
		}

		if err := symbol.AppendMeasurement(*signal.bookReading(managed, tickSize)); err != nil {
			emitErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"depthflow: failed to emit book reading",
				err,
			))
		}
	})

	if emitErr != nil {
		return emitErr
	}

	for trade := range symbol.MarketTrades(types.SourceDepthFlow) {
		if !validTrade(trade) {
			continue
		}

		if err := symbol.AppendMeasurement(*signal.tradeReading(trade)); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"depthflow: failed to emit trade reading",
				err,
			))
		}
	}

	return nil
}

/*
bookReading folds the current managed book into the per-symbol flow window and
returns one measurement. An immature window is still an honest zero reading.
*/
func (signal *Signal) bookReading(
	managed *spotbook.Book,
	tickSize float64,
) *types.Measurement {
	if signal.sample == nil || signal.bookflow == nil {
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

	bookflowInput, ready, maturity, err := signal.sample.MeasureBook(input)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure book",
			err,
		))

		return immatureReading(managed.Name, observedAt)
	}

	if !ready {
		return immatureReading(managed.Name, observedAt)
	}

	output, err := signal.bookflow.Measure(bookflowInput)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure bookflow",
			err,
		))

		return immatureReading(managed.Name, observedAt)
	}

	measurement := signal.frame(managed.Name, observedAt, output, maturity)

	if bestBid != nil && bestAsk != nil {
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
	}

	return measurement
}

/*
tradeReading applies one trade event to the shared flow window and returns one
measurement.
*/
func (signal *Signal) tradeReading(row kraken.TradeData) *types.Measurement {
	if signal.sample == nil || signal.bookflow == nil {
		return immatureReading(row.Symbol, row.Timestamp)
	}

	input, _, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     flow.TradeSide(row.Side),
		At:       row.Timestamp,
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure trade",
			err,
		))

		return immatureReading(row.Symbol, row.Timestamp)
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure bookflow",
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
immatureReading is the honest zero reading for a frame the flow window cannot
score yet.
*/
func immatureReading(symbol string, at time.Time) *types.Measurement {
	return &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceDepthFlow,
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
frame converts a bookflow output into the shared Measurement shape.
*/
func (signal *Signal) frame(
	symbol string,
	at time.Time,
	output equation.BookflowOutput,
	maturity float64,
) *types.Measurement {
	loaded := normalizedBookflowScore(types.MetricLoadedScore, output.LoadedScore)
	spoof := normalizedBookflowScore(types.MetricSpoofScore, output.SpoofScore)
	thin := normalizedBookflowScore(types.MetricThinScore, output.ThinScore)
	neutral := normalizedBookflowScore(types.MetricNeutralScore, output.NeutralScore)

	if loaded == nil || spoof == nil || thin == nil || neutral == nil {
		panic("depthflow: bookflow output outside its defined metric domain")
	}

	if !output.Ready {
		loaded = nil
		spoof = nil
		thin = nil
		neutral = nil
	}

	measurement := &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceDepthFlow,
		Symbol:   symbol,
		At:       at,
		Maturity: maturity,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricLoadedScore, types.SideNone): {
				Raw:        output.LoadedScore,
				Normalized: loaded,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSpoofScore, types.SideNone): {
				Raw:        output.SpoofScore,
				Normalized: spoof,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricThinScore, types.SideNone): {
				Raw:        output.ThinScore,
				Normalized: thin,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricNeutralScore, types.SideNone): {
				Raw:        output.NeutralScore,
				Normalized: neutral,
				Unit:       types.UnitDimensionless,
			},
		},
	}

	return measurement
}

const maxBookImbalanceContrast = 2.0

/*
normalizedBookflowScore preserves the equation's bounded depth fractions. A
spoof score is the absolute difference of two imbalances in [-1, 1], so its
domain-derived maximum contrast is two.
*/
func normalizedBookflowScore(
	metric types.MetricType,
	raw float64,
) *float64 {
	maximum := 1.0

	if metric == types.MetricSpoofScore {
		maximum = maxBookImbalanceContrast
	}

	if raw < 0 || raw > maximum {
		return nil
	}

	value := raw

	if metric == types.MetricSpoofScore {
		value = raw / maxBookImbalanceContrast
	}

	return &value
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal != nil && signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
