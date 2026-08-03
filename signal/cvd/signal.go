package cvd

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is the Absorption perspective, measuring signed aggressor flow against
price response. Categories belong in logic; this signal emits numerical scores
only.
*/
type Signal struct {
	status         types.Status
	ctx            context.Context
	cancel         context.CancelFunc
	api            *websocket.API
	sample         *algorithm.TradeFlowSample
	flow           *equation.Flow
	midpoints      map[string][]midpointObservation
	planner        *strategy.Planner
	ui             chan []byte
	subscriptions  map[string]*types.Subscription[any]
	subscribers    *sync.Map
	subscriptionMu sync.Mutex
	lastTrade      map[string]tradeCursor
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

type midpointObservation struct {
	at    time.Time
	price float64
}

/*
NewSignal creates the CVD perspective with independent rolling state for each
symbol so one market's aggressor history cannot leak into another's evidence.
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
		sample:        algorithm.NewTradeFlowSample(),
		flow:          equation.NewFlow(),
		midpoints:     make(map[string][]midpointObservation),
		planner:       planner,
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
		lastTrade:     make(map[string]tradeCursor),
	}

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCVD)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return utils.Subscribe(
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) ensureProcessors() (*algorithm.TradeFlowSample, *equation.Flow) {
	if signal.sample == nil {
		signal.sample = algorithm.NewTradeFlowSample()
	}

	if signal.flow == nil {
		signal.flow = equation.NewFlow()
	}

	return signal.sample, signal.flow
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourceCVD,
						signal.Measure(thesis),
						types.Stamp{
							At:     time.Now(),
							Entity: types.MarketTrade,
							Source: types.SourceCVD,
						},
					)

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers := thesis.MarketTickers()
	trades := thesis.MarketTrades()

	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	for _, row := range tickers {
		signal.observeMidpoint(row)
	}

	for _, row := range trades {
		if !validTrade(row) || signal.seenTrade(row) {
			continue
		}

		midpoint, index, exists := signal.midpointAt(row.Symbol, row.Timestamp)

		if !exists {
			continue
		}

		tradeMeasurements, err := signal.measureTrade(row, midpoint.price)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"cvd: failed to measure trade",
				err,
			))
			continue
		}

		signal.commitTrade(row)
		signal.commitMidpoint(row.Symbol, index)

		measurements = append(measurements, tradeMeasurements...)

		if row.Symbol == types.Focus() {
			out = append(out, tradeMeasurements...)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", out))
	}

	return measurements
}

func (signal *Signal) observeMidpoint(row kraken.TickerData) {
	if row.Symbol == "" || row.Timestamp.IsZero() || row.Bid == nil || row.Ask == nil {
		return
	}

	bid := row.Bid.Float64()
	ask := row.Ask.Float64()

	if bid <= 0 || ask <= bid || math.IsNaN(bid) || math.IsNaN(ask) ||
		math.IsInf(bid, 0) || math.IsInf(ask, 0) {
		return
	}

	if signal.midpoints == nil {
		signal.midpoints = make(map[string][]midpointObservation)
	}

	observations := signal.midpoints[row.Symbol]
	observation := midpointObservation{at: row.Timestamp, price: (bid + ask) / 2}
	insertAt := len(observations)

	for index, existing := range observations {
		if existing.at.Equal(observation.at) {
			observations[index] = observation
			signal.midpoints[row.Symbol] = observations

			return
		}

		if existing.at.After(observation.at) {
			insertAt = index
			break
		}
	}

	observations = append(observations, midpointObservation{})
	copy(observations[insertAt+1:], observations[insertAt:])
	observations[insertAt] = observation
	signal.midpoints[row.Symbol] = observations
}

func (signal *Signal) midpointAt(
	symbol string,
	at time.Time,
) (midpointObservation, int, bool) {
	observations := signal.midpoints[symbol]

	for index := len(observations) - 1; index >= 0; index-- {
		if !observations[index].at.After(at) {
			return observations[index], index, true
		}
	}

	return midpointObservation{}, 0, false
}

func (signal *Signal) commitMidpoint(symbol string, index int) {
	observations := signal.midpoints[symbol]

	if index <= 0 || index >= len(observations) {
		return
	}

	signal.midpoints[symbol] = observations[index:]
}

func validTrade(row kraken.TradeData) bool {
	price := row.Price.Float64()

	return row.Symbol != "" && !row.Timestamp.IsZero() && price > 0 && row.Qty > 0 &&
		!math.IsNaN(price) && !math.IsInf(price, 0) && !math.IsNaN(row.Qty) &&
		!math.IsInf(row.Qty, 0) && (row.Side == "buy" || row.Side == "sell")
}

func (signal *Signal) seenTrade(row kraken.TradeData) bool {
	previous := signal.lastTrade[row.Symbol]

	if row.Timestamp.Before(previous.at) {
		return true
	}

	if row.Timestamp.After(previous.at) {
		return false
	}

	_, seen := previous.ids[row.TradeID]

	return seen
}

func (signal *Signal) commitTrade(row kraken.TradeData) {
	previous := signal.lastTrade[row.Symbol]

	if row.Timestamp.After(previous.at) {
		previous = tradeCursor{
			at:  row.Timestamp,
			ids: make(map[int64]struct{}),
		}
	}

	if previous.ids == nil {
		previous.ids = make(map[int64]struct{})
	}

	previous.ids[row.TradeID] = struct{}{}
	signal.lastTrade[row.Symbol] = previous
}

/*
measureTrade separates execution notional from midpoint response before
classifying one aggressor observation through the adaptive CVD window.
*/
func (signal *Signal) measureTrade(
	row kraken.TradeData,
	midpoint float64,
) ([]*types.Measurement, error) {
	sample, flow := signal.ensureProcessors()

	input, ready, err := sample.Measure(algorithm.TradeFlowInput{
		Symbol:        row.Symbol,
		Price:         row.Price.Float64(),
		ResponsePrice: midpoint,
		Quantity:      row.Qty,
		Side:          row.Side,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := flow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	return signal.cvdMeasurements(row, output, input.TradeCount), nil
}

/*
cvdMeasurements maps signed-flow output into one source×symbol row whose
Metrics map preserves each reading's unit and normalization.
*/
func (signal *Signal) cvdMeasurements(
	row kraken.TradeData,
	output equation.FlowOutput,
	evidenceCount int,
) []*types.Measurement {
	measurement := &types.Measurement{
		Source:   types.SourceCVD,
		Symbol:   row.Symbol,
		At:       row.Timestamp,
		Validity: types.ObservationValidity(evidenceCount),
		Metrics:  make(map[string]types.MetricSample, 7),
	}
	measurement.Metrics[types.MetricKey(types.MetricAbsorption, types.SideNone)] = types.MetricSample{Raw: output.Absorption, Normalized: types.NormalizeFinite(output.Absorption), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricDrive, types.SideNone)] = types.MetricSample{Raw: output.Drive, Normalized: types.NormalizeFinite(output.Drive), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricBalance, types.SideNone)] = types.MetricSample{Raw: output.Balance, Normalized: types.NormalizeFinite(output.Balance), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStarvation, types.SideNone)] = types.MetricSample{Raw: output.Starvation, Normalized: types.NormalizeFinite(output.Starvation), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStrength, types.SideNone)] = types.MetricSample{Raw: output.Value, Normalized: types.NormalizeFinite(output.Value), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNetFraction, types.SideNone)] = types.MetricSample{Raw: output.NetFraction, Normalized: types.NormalizeFinite(output.NetFraction), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNet, types.SideNone)] = types.MetricSample{Raw: output.Net, Unit: types.UnitQuoteCurrency}

	return []*types.Measurement{measurement}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
