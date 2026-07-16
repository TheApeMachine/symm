package depthflow

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
DepthFlow is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Categories belong in logic; this
signal emits numerical scores only.
*/
type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	book     *Book
	trade    *Trade
	sample   *flow.Sample
	bookflow *equation.Bookflow
}

func NewSignal(
	ctx context.Context, api *websocket.API, instrument *broker.Instrument,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:      ctx,
		cancel:   cancel,
		book:     NewBook(ctx, api, instrument),
		trade:    NewTrade(ctx, api),
		sample:   flow.NewSample(),
		bookflow: equation.NewBookflow(),
	}
}

/*
Capture freezes book and trade journals at one planner boundary so their event
ranges can be merged without admitting later observations.
*/
func (signal *Signal) Capture(at time.Time) error {
	if err := signal.book.cache.Capture(at); err != nil {
		return err
	}

	return signal.trade.cache.Capture(at)
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	bookBatch, err := signal.book.cache.Batch(thesis.At)

	if err != nil {
		errnie.Error(err)
		return thesis
	}

	tradeBatch, err := signal.trade.cache.Batch(thesis.At)

	if err != nil {
		errnie.Error(err)
		return thesis
	}
	events := depthEvents(bookBatch.Rows, tradeBatch.Rows)
	out := make([]*types.Measurement, 0, len(events))

	for _, event := range events {
		if event.Stream == "book" {
			out = append(out, signal.measureBook(event.Row.(kraken.BookData))...)
			continue
		}

		out = append(out, signal.measureTrade(event.Row.(kraken.TradeData))...)
	}

	if err := signal.book.cache.Commit(bookBatch); err != nil {
		errnie.Error(err)
		return thesis
	}

	if err := signal.trade.cache.Commit(tradeBatch); err != nil {
		errnie.Error(err)
		return thesis
	}

	thesis.Signals.Store("depthflow.books", bookBatch.Rows)
	thesis.Signals.Store("depthflow.trades", tradeBatch.Rows)
	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

/*
measureBook applies one book event to the shared flow sample and emits the
resulting measurements only after both sample and equation report readiness.
*/
func (signal *Signal) measureBook(row kraken.BookData) []*types.Measurement {
	if row.Symbol == "" || row.PriceIncrement.Sign() <= 0 {
		return nil
	}

	bids, asks, err := signal.book.levels(row)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	input, ready, maturity, err := signal.sample.MeasureBook(flow.BookInput{
		Symbol:   row.Symbol,
		TickSize: row.PriceIncrement.Float64(),
		Bids:     bids,
		Asks:     asks,
	})

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if !ready {
		return nil
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if !output.Ready {
		return nil
	}

	return signal.measurements(row.Symbol, row.Timestamp, output, maturity)
}

/*
measureTrade applies one trade event to the shared flow sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureTrade(row kraken.TradeData) []*types.Measurement {
	if row.Symbol == "" || row.Price.Sign() <= 0 || row.Qty <= 0 {
		return nil
	}

	input, ready, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     row.Side,
	})

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if !ready {
		return nil
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if !output.Ready {
		return nil
	}

	return signal.measurements(row.Symbol, row.Timestamp, output, maturity)
}

/*
depthEvents merges book and trade batches by event time. Trades precede books
at equal timestamps so a publishing book observation includes simultaneous tape.
*/
func depthEvents(
	books []kraken.BookData,
	trades []kraken.TradeData,
) []types.Event {
	events := make([]types.Event, 0, len(books)+len(trades))

	for index, row := range trades {
		events = append(events, types.Event{
			Stream:   "trade",
			Priority: 0,
			Sequence: uint64(index + 1),
			At:       row.Timestamp,
			Symbol:   row.Symbol,
			Row:      row,
		})
	}

	for index, row := range books {
		events = append(events, types.Event{
			Stream:   "book",
			Priority: 1,
			Sequence: uint64(index + 1),
			At:       row.Timestamp,
			Symbol:   row.Symbol,
			Row:      row,
		})
	}

	types.OrderEvents(events)

	return events
}

/*
measurements converts a bookflow calculator output into the shared
Measurement shape, so both the book-driven and trade-driven observation paths
emit the same metric set for a symbol.
*/
func (signal *Signal) measurements(
	symbol string, at time.Time, output equation.BookflowOutput, maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	scale := types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    at,
		Through: at,
	}
	specs := []struct {
		metric types.MetricType
		raw    float64
	}{
		{types.MetricLoadedScore, output.LoadedScore},
		{types.MetricSpoofScore, output.SpoofScore},
		{types.MetricThinScore, output.ThinScore},
		{types.MetricNeutralScore, output.NeutralScore},
		{types.MetricStrength, output.Strength},
		{types.MetricValue, output.Value},
	}
	measurements := make([]*types.Measurement, 0, len(specs))

	for _, spec := range specs {
		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceDepthFlow,
			Stream:     types.DepthFlow,
			Metric:     spec.metric,
			Subject:    types.SubjectBookImbalance,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        spec.raw,
			Normalized: types.NormalizeFinite(spec.raw),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		})
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
