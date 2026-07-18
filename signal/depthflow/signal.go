package depthflow

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
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
	sample   *flow.Sample
	bookflow *equation.Bookflow
	ui       chan []byte
}

func NewSignal(
	ctx context.Context, api *websocket.API, instrument *broker.Instrument, ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:      ctx,
		cancel:   cancel,
		sample:   flow.NewSample(),
		bookflow: equation.NewBookflow(),
		ui:       ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.WireMeasurements(measurements),
	}.Marshal():
	default:
	}
}

/*
Measure supports direct replay against legacy signal-local journals. The live
runtime uses Calculate with the central immutable market cut.
*/
func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, err := signal.Calculate(thesis.Market())

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return measurements
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	events := depthEvents(frame.Books, frame.Trades)
	out := make([]*types.Measurement, 0, len(events))

	for _, event := range events {
		if event.Stream == "book" {
			measurements := signal.measureBook(event.Row.(kraken.BookData))
			out = append(out, measurements...)
			signal.Publish(measurements)
			continue
		}

		measurements := signal.measureTrade(event.Row.(kraken.TradeData))
		out = append(out, measurements...)
	}

	signal.Publish(out)
	return out, nil
}

/*
measureBook applies one book event to the shared flow sample and emits the
resulting measurements only after both sample and equation report readiness.
*/
func (signal *Signal) measureBook(row kraken.BookData) []*types.Measurement {
	if row.Symbol == "" {
		return nil
	}

	if row.PriceIncrement == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"depthflow: price increment required for "+row.Symbol,
			nil,
		))

		return nil
	}

	if row.PriceIncrement.Sign() <= 0 {
		return nil
	}

	bids, asks, err := types.BookLevels(row)

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
