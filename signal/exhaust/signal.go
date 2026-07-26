package exhaust

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

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
	*types.Actor
	thesis *types.Thesis
	ctx    context.Context
	cancel context.CancelFunc
	sample *algorithm.DecaySample
	decay  *equation.Decay
	ui     chan []byte
}

/*
NewSignal constructs the market-wide exhaustion observer. Position inventory is
deliberately absent: the signal measures both hypothetical exit sides, leaving
the consumer to select the side matching its position.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		sample: algorithm.NewDecaySample(),
		decay:  equation.NewDecay(),
		ui:     ui,
	}

	signal.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {Topic: "thesis", Fn: signal.onTicker},
		"book":   {Topic: "thesis", Fn: signal.onBook},
		"trade":  {Topic: "thesis", Fn: signal.onTrade},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceExhaustion)
}

/*
Initialize wires ticker, book, and trade ingress from Live.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
		types.Topic{Name: "book", Actor: live},
		types.Topic{Name: "trade", Actor: live},
	)
}

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return types.SignalResult{Source: types.SourceExhaustion, Status: types.SignalSkip}
	}

	if len(measurements) == 0 {
		return types.SignalResult{Source: types.SourceExhaustion, Status: types.SignalSkip}
	}

	signal.thesis.Publish(types.SourceExhaustion, measurements)

	return types.SignalResult{Source: types.SourceExhaustion, Measurements: measurements, Status: types.SignalReady}
}

func (signal *Signal) onBook(message any) any {
	rows := message.(*kraken.Book).Data
	measurements, err := signal.Calculate(nil, nil, rows)

	if err != nil {
		errnie.Error(err)
		return types.SignalResult{Source: types.SourceExhaustion, Status: types.SignalSkip}
	}

	if len(measurements) == 0 {
		return types.SignalResult{Source: types.SourceExhaustion, Status: types.SignalSkip}
	}

	signal.thesis.Publish(types.SourceExhaustion, measurements)

	return types.SignalResult{Source: types.SourceExhaustion, Measurements: measurements, Status: types.SignalReady}
}

func (signal *Signal) onTrade(message any) any {
	rows := message.(*kraken.Trade).Data
	measurements, err := signal.Calculate(nil, rows, nil)

	if err != nil {
		errnie.Error(err)
		return types.SignalResult{Source: types.SourceExhaustion, Status: types.SignalSkip}
	}

	if len(measurements) == 0 {
		return types.SignalResult{Source: types.SourceExhaustion, Status: types.SignalSkip}
	}

	signal.thesis.Publish(types.SourceExhaustion, measurements)

	return types.SignalResult{Source: types.SourceExhaustion, Measurements: measurements, Status: types.SignalReady}
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	events := exhaustEvents(books, trades)
	out, err := types.MeasureEventsParallel(events, func(event types.Event) ([]*types.Measurement, error) {
		if event.Stream == "book" {
			return signal.measureBook(event.Row.(kraken.BookData))
		}

		return signal.measureTrade(event.Row.(kraken.TradeData))
	})

	if err != nil {
		return nil, err
	}

	select {
	case signal.ui <- datura.Map[any]{
		"measurements": out,
	}.Marshal():
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"wire: ui channel saturated; dropped measurements",
			nil,
		))
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

	return signal.frame(row.Symbol, row.Timestamp, output, maturity), nil
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
		Side:     flow.TradeSide(row.Side),
		At:       row.Timestamp,
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

	return signal.frame(row.Symbol, row.Timestamp, output, maturity), nil
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
frame converts a decay calculator output into the shared Measurement shape, so
both the book-driven and trade-driven observation paths emit the same metric
set for a symbol.
*/
func (signal *Signal) frame(
	symbol string, at time.Time, output equation.DecayOutput, maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	measurement := &types.Measurement{
		Source:       types.SourceExhaustion,
		Symbol:       symbol,
		At:           at,
		ObservedFrom: at,
		Maturity:     maturity,
		Validity:     validity,
		Metrics:      make(map[string]types.MetricSample, 16),
	}

	signal.putSideMetrics(measurement, output.Long, types.SideBuy)
	signal.putSideMetrics(measurement, output.Short, types.SideSell)

	return []*types.Measurement{measurement}
}

/*
putSideMetrics preserves which held position side the decay evidence would
advise exiting, rather than merging contradictory long and short conditions.
*/
func (signal *Signal) putSideMetrics(
	measurement *types.Measurement,
	output equation.DecaySideOutput,
	side types.MeasurementSide,
) {
	measurement.Metrics[types.MetricKey(types.MetricMechanical, side)] = types.MetricSample{Raw: output.Mechanical, Normalized: types.NormalizeFinite(output.Mechanical), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricThermal, side)] = types.MetricSample{Raw: output.Thermal, Normalized: types.NormalizeFinite(output.Thermal), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricFragile, side)] = types.MetricSample{Raw: output.Fragile, Normalized: types.NormalizeFinite(output.Fragile), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricReversal, side)] = types.MetricSample{Raw: output.Reversal, Normalized: types.NormalizeFinite(output.Reversal), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricUrgency, side)] = types.MetricSample{Raw: output.Urgency, Normalized: types.NormalizeFinite(output.Urgency), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStrength, side)] = types.MetricSample{Raw: output.Strength, Normalized: types.NormalizeFinite(output.Strength), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricValue, side)] = types.MetricSample{Raw: output.Value, Normalized: types.NormalizeFinite(output.Value), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricCategory, side)] = types.MetricSample{Raw: output.Category, Normalized: types.NormalizeFinite(output.Category), Unit: types.UnitDimensionless}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
