package exhaust

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
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
Exhaust is the Exit Thesis perspective, tracking microstructure decay to advise
on the urgency of closing an open position. Categories belong in logic; this
signal emits numerical scores only.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	sample *algorithm.DecaySample
	decay  *equation.Decay
	ui     chan []byte
}

func NewSignal(
	ctx context.Context, api *websocket.API, instrument *broker.Instrument, ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		sample: algorithm.NewDecaySample(),
		decay:  equation.NewDecay(),
		ui:     ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": measurements,
	}.Marshal():
	default:
	}
}

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
	events := exhaustEvents(frame.Books, frame.Trades)
	out := make([]*types.Measurement, 0, 8*len(events))

	for _, event := range events {
		if event.Stream == "book" {
			measurements := signal.measureBook(event.Row.(kraken.BookData))
			out = append(out, measurements...)
			signal.Publish(measurements)
			continue
		}

		measurements := signal.measureTrade(event.Row.(kraken.TradeData))
		out = append(out, measurements...)
		signal.Publish(measurements)
	}

	return out, nil
}

/*
measureBook applies one book event to the shared decay sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureBook(row kraken.BookData) []*types.Measurement {
	if row.Symbol == "" || row.PriceIncrement.Sign() <= 0 {
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

	output, err := signal.decay.Measure(input)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return signal.measurements(row.Symbol, row.Timestamp, output, maturity)
}

/*
measureTrade applies one trade event to the shared decay sample at its causal
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

	output, err := signal.decay.Measure(input)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return signal.measurements(row.Symbol, row.Timestamp, output, maturity)
}

/*
exhaustEvents merges book and trade batches by event time. Trades precede books
at equal timestamps so the book state includes simultaneous executed pressure.
*/
func exhaustEvents(
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
measurements converts a decay calculator output into the shared Measurement
shape, so both the book-driven and trade-driven observation paths emit the
same metric set for a symbol.
*/
func (signal *Signal) measurements(
	symbol string, at time.Time, output equation.DecayOutput, maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	specs := []struct {
		metric types.MetricType
		raw    float64
	}{
		{types.MetricMechanical, output.Mechanical},
		{types.MetricThermal, output.Thermal},
		{types.MetricFragile, output.Fragile},
		{types.MetricReversal, output.Reversal},
		{types.MetricUrgency, output.Urgency},
		{types.MetricStrength, output.Strength},
		{types.MetricValue, output.Value},
		{types.MetricCategory, output.Category},
	}
	measurements := make([]*types.Measurement, 0, len(specs))

	for _, spec := range specs {
		measurements = append(measurements, &types.Measurement{
			Source:       types.SourceExhaustion,
			Metric:       spec.metric,
			Stream:       types.Exhaust,
			Symbol:       symbol,
			At:           at,
			ObservedFrom: at,
			Unit:         types.UnitDimensionless,
			Raw:          spec.raw,
			Normalized:   types.NormalizeFinite(spec.raw),
			Maturity:     maturity,
			Validity:     validity,
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
