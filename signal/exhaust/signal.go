package exhaust

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal tracks side-specific microstructure decay that can advise whether a long
or short position should exit. It emits numerical family scores, their fused
urgency, and the winning numerical family identifier for downstream logic.
*/
type Signal struct {
	tickerIn chan []kraken.TickerData
	bookIn   chan []kraken.BookData
	tradeIn  chan []kraken.TradeData
	ack     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	sample   *algorithm.DecaySample
	decay    *equation.Decay
	ui       chan []byte
}

/*
NewSignal constructs the market-wide exhaustion observer. Position inventory is
deliberately absent: the signal measures both hypothetical exit sides, leaving
the consumer to select the side matching its position.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		tickerIn: make(chan []kraken.TickerData, 64),
		bookIn:   make(chan []kraken.BookData, 64),
		tradeIn:  make(chan []kraken.TradeData, 64),
		ack:     make(chan struct{}, 256),
		ctx:      ctx,
		cancel:   cancel,
		sample:   algorithm.NewDecaySample(),
		decay:    equation.NewDecay(),
		ui:       ui,
	}

	return signal
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.ForPublish(measurements),
	}.Marshal():
	default:
	}
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	events := exhaustEvents(books, trades)
	out := make([]*types.Measurement, 0, 16*len(events))

	for _, event := range events {
		if event.Stream == "book" {
			measurements, err := signal.measureBook(event.Row.(kraken.BookData))

			if err != nil {
				continue
			}

			out = append(out, measurements...)
			continue
		}

		measurements, err := signal.measureTrade(event.Row.(kraken.TradeData))

		if err != nil {
			continue
		}

		out = append(out, measurements...)
	}

	return out, nil
}

/*
measureBook applies one book event to the shared decay sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureBook(
	row kraken.BookData,
) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Timestamp.IsZero() || row.PriceIncrement == nil {
		return nil, nil
	}

	if row.PriceIncrement.Sign() <= 0 {
		return nil, nil
	}

	bids, asks, err := kraken.BookLevels(row)

	if err != nil {
		return nil, err
	}

	input, ready, maturity, err := signal.sample.MeasureBook(flow.BookInput{
		Symbol:   row.Symbol,
		TickSize: row.PriceIncrement.Float64(),
		Bids:     bids,
		Asks:     asks,
	})

	if err != nil {
		return nil, err
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.decay.Measure(input)

	if err != nil {
		return nil, err
	}

	return signal.measurements(row.Symbol, row.Timestamp, output, maturity), nil
}

/*
measureTrade applies one trade event to the shared decay sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureTrade(
	row kraken.TradeData,
) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Timestamp.IsZero() || row.Price.Sign() <= 0 ||
		row.Qty <= 0 || row.Side != "buy" && row.Side != "sell" {
		return nil, nil
	}

	input, ready, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     row.Side,
	})

	if err != nil {
		return nil, err
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.decay.Measure(input)

	if err != nil {
		return nil, err
	}

	return signal.measurements(row.Symbol, row.Timestamp, output, maturity), nil
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
	measurements := signal.sideMeasurements(
		symbol, at, output.Long, types.SideBuy, maturity,
	)

	return append(measurements, signal.sideMeasurements(
		symbol, at, output.Short, types.SideSell, maturity,
	)...)
}

/*
sideMeasurements preserves which held position side the decay evidence would
advise exiting, rather than merging contradictory long and short conditions.
*/
func (signal *Signal) sideMeasurements(
	symbol string,
	at time.Time,
	output equation.DecaySideOutput,
	side types.MeasurementSide,
	maturity float64,
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
			Side:         side,
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
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}

/*
Tickers returns the ticker ingress channel.
*/
func (signal *Signal) Tickers() chan []kraken.TickerData {
	return signal.tickerIn
}

/*
Books returns the book ingress channel.
*/
func (signal *Signal) Books() chan []kraken.BookData {
	return signal.bookIn
}

/*
Trades returns the trade ingress channel.
*/
func (signal *Signal) Trades() chan []kraken.TradeData {
	return signal.tradeIn
}


/*
Ack signals that one ingress frame finished Calculate so Crypto can barrier
before draining outs.
*/
func (signal *Signal) Ack() <-chan struct{} {
	return signal.ack
}

/*
Measure consumes ingress channels and sends measurements on out.
*/
func (signal *Signal) Measure() chan []*types.Measurement {
	out := make(chan []*types.Measurement, 64)

	go func() {
		defer close(out)

		for {
			select {
			case <-signal.ctx.Done():
				return
			case rows := <-signal.tickerIn:
				measured, err := signal.Calculate(rows, nil, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.bookIn:
				measured, err := signal.Calculate(nil, nil, rows)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.tradeIn:
				measured, err := signal.Calculate(nil, rows, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			}
		}
	}()

	return out
}
