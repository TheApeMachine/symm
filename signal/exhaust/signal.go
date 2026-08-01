package exhaust

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal tracks side-specific microstructure decay that can advise whether a long
or short position should exit. It emits numerical family scores, their fused
urgency, and the winning numerical family identifier for downstream logic.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
}

/*
NewSignal constructs the market-wide exhaustion observer. Position inventory is
deliberately absent: the signal measures both hypothetical exit sides, leaving
the consumer to select the side matching its position.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	planner *strategy.Planner,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		planner:       planner,
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}
	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceExhaustion)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	if signal.subscribers == nil {
		signal.subscribers = &sync.Map{}
	}

	subscribers, ok := signal.subscribers.LoadOrStore(
		channel, []*types.Subscription[any]{subscription},
	)

	if ok && subscribers != nil {
		signal.subscribers.Store(
			channel, append(subscribers.([]*types.Subscription[any]), subscription),
		)
	}

	return subscription
}

func (signal *Signal) run() {
	thesisSubscription := signal.subscriptions["thesis"]

	if thesisSubscription == nil {
		return
	}

	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-thesisSubscription.Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourceExhaustion,
						signal.Measure(thesis),
						types.Stamp{At: time.Now(), Entity: types.MarketTrade},
					)

					subscribers, ok := signal.subscribers.Load(signal.Name())

					if ok && subscribers != nil {
						for _, subscriber := range subscribers.([]*types.Subscription[any]) {
							subscriber.Send(thesis)
						}
					}
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	if _, ok := thesis.Causal.Load("signal:exhaust:sample"); !ok {
		thesis.Causal.Store("signal:exhaust:sample", algorithm.NewDecaySample())
	}

	if _, ok := thesis.Causal.Load("signal:exhaust:decay"); !ok {
		thesis.Causal.Store("signal:exhaust:decay", equation.NewDecay())
	}

	_, trades, _ := thesis.Market()
	events := exhaustEvents(nil, trades)
	out, err := types.MeasureEventsParallel(events, func(event types.Event) ([]*types.Measurement, error) {
		if event.Stream == "book" {
			return signal.measureBook(thesis, event.Row.(kraken.BookData))
		}

		return signal.measureTrade(thesis, event.Row.(kraken.TradeData))
	})

	if err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "exhaust: failed to measure events", err))
		return nil
	}

	var focusMeasurements []*types.Measurement

	for _, measurement := range out {
		if measurement.Symbol == types.Focus() {
			focusMeasurements = append(focusMeasurements, measurement)
		}
	}

	if len(focusMeasurements) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", focusMeasurements))
	}

	return out
}

/*
measureBook applies one book event to the shared decay sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureBook(
	thesis *types.Thesis,
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

	found, _ := thesis.Causal.Load("signal:exhaust:sample")
	input, ready, maturity, err := found.(*algorithm.DecaySample).MeasureBook(flow.BookInput{
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

	found, _ = thesis.Causal.Load("signal:exhaust:decay")
	output, err := found.(*equation.Decay).Measure(input)

	if err != nil {
		return nil, err
	}

	return signal.frame(thesis, row.Symbol, row.Timestamp, output, maturity), nil
}

/*
measureTrade applies one trade event to the shared decay sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureTrade(
	thesis *types.Thesis,
	row kraken.TradeData,
) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Timestamp.IsZero() || row.Price.Sign() <= 0 ||
		row.Qty <= 0 || row.Side != "buy" && row.Side != "sell" {
		return nil, nil
	}

	found, _ := thesis.Causal.Load("signal:exhaust:sample")
	input, ready, maturity, err := found.(*algorithm.DecaySample).MeasureTrade(flow.TradeInput{
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

	found, _ = thesis.Causal.Load("signal:exhaust:decay")
	output, err := found.(*equation.Decay).Measure(input)

	if err != nil {
		return nil, err
	}

	return signal.frame(thesis, row.Symbol, row.Timestamp, output, maturity), nil
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
	thesis *types.Thesis,
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
		Maturity:     float64(thesis.Tick),
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
