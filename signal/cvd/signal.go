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
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	sample        *algorithm.TradeFlowSample
	flow          *equation.Flow
	midpoints     map[string][]midpointObservation
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	lastTrade     map[string]tradeCursor
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

type midpointObservation struct {
	at    time.Time
	price float64
}

const minimumPriceResponseObservations = 2

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
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.AppendMeasurements(
							types.SourceCVD,
							measurements...,
						)

						thesis.Readiness.Stamp(types.SourceCVD)
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
					}

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
	absorption := normalizedFlowMetric(
		types.MetricAbsorption, output.Absorption, evidenceCount,
	)
	drive := normalizedFlowMetric(types.MetricDrive, output.Drive, evidenceCount)
	balance := normalizedFlowMetric(types.MetricBalance, output.Balance, evidenceCount)
	starvation := normalizedFlowMetric(
		types.MetricStarvation, output.Starvation, evidenceCount,
	)
	strength := normalizedFlowMetric(types.MetricStrength, output.Value, evidenceCount)
	netFraction := normalizedFlowMetric(
		types.MetricNetFraction, output.NetFraction, evidenceCount,
	)
	net := normalizedSignedNet(output.Net, output.NetFraction, evidenceCount)
	validity := types.ObservationValidity(evidenceCount)

	if validity.State == types.ValidityValid &&
		(absorption == nil || drive == nil || balance == nil || starvation == nil ||
			strength == nil || netFraction == nil || net == nil) {
		validity.State = types.ValidityInvalid
		validity.Reason = "flow normalization contract violated"
	}

	measurement := &types.Measurement{
		Source:   types.SourceCVD,
		Symbol:   row.Symbol,
		At:       row.Timestamp,
		Validity: validity,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricAbsorption, types.SideNone): {
				Raw:        output.Absorption,
				Normalized: absorption,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricDrive, types.SideNone): {
				Raw:        output.Drive,
				Normalized: drive,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricBalance, types.SideNone): {
				Raw:        output.Balance,
				Normalized: balance,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStarvation, types.SideNone): {
				Raw:        output.Starvation,
				Normalized: starvation,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStrength, types.SideNone): {
				Raw:        output.Value,
				Normalized: strength,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricNetFraction, types.SideNone): {
				Raw:        output.NetFraction,
				Normalized: netFraction,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricNet, types.SideNone): {
				Raw:        output.Net,
				Normalized: net,
				Unit:       types.UnitQuoteCurrency,
			},
		},
	}

	return []*types.Measurement{measurement}
}

/*
normalizedFlowMetric accepts only the bounded fractions produced by Flow.
Price-response families need at least two response prices; balance, strength,
and net fraction are defined by the first observed aggressor split.
*/
func normalizedFlowMetric(
	metric types.MetricType,
	raw float64,
	evidenceCount int,
) *float64 {
	if math.IsNaN(raw) || math.IsInf(raw, 0) || raw < 0 || raw > 1 {
		return nil
	}

	if evidenceCount <= 0 {
		return nil
	}

	if evidenceCount < minimumPriceResponseObservations &&
		(metric == types.MetricAbsorption || metric == types.MetricDrive ||
			metric == types.MetricStarvation) {
		return nil
	}

	value := raw

	return &value
}

/*
normalizedSignedNet restores direction to Flow's gross-notional fraction. The
raw quote-currency net remains available while the normalized reading is the
signed share of actually executed gross notional.
*/
func normalizedSignedNet(raw, netFraction float64, evidenceCount int) *float64 {
	if evidenceCount <= 0 {
		return nil
	}

	if math.IsNaN(raw) || math.IsInf(raw, 0) || netFraction < 0 || netFraction > 1 ||
		math.IsNaN(netFraction) || math.IsInf(netFraction, 0) {
		return nil
	}

	if raw == 0 && netFraction != 0 {
		return nil
	}

	value := math.Copysign(netFraction, raw)

	if raw == 0 {
		value = 0
	}

	return &value
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
