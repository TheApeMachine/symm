package cvd

import (
	"context"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

/*
Signal is the Absorption perspective, measuring signed aggressor flow against
price response. Categories belong in logic; this signal emits numerical scores
only.
*/
type Signal struct {
	status    atomic.Value
	ctx       context.Context
	cancel    context.CancelFunc
	api       *websocket.API
	sample    *algorithm.TradeFlowSample
	flow      *equation.Flow
	midpoints *sync.Map
	ui        chan []byte
	thesis    *types.Thesis
	semaphore chan struct{}
	lastTrade *sync.Map
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
	ui chan []byte,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:       ctx,
		cancel:    cancel,
		api:       api,
		sample:    algorithm.NewTradeFlowSample(),
		flow:      equation.NewFlow(),
		midpoints: &sync.Map{},
		ui:        ui,
		thesis:    thesis,
		semaphore: make(chan struct{}, 1),
		lastTrade: &sync.Map{},
	}

	signal.status.Store(types.INITIALIZING)
	signal.thesis.Subscribe(types.SourceCVD, signal.semaphore)
	signal.status.Store(types.READY)
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
	return signal.status.Load().(types.Status)
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
			case <-signal.semaphore:
				signal.status.Store(types.BUSY)
				measurements := signal.Measure(signal.thesis)

				if len(measurements) > 0 {
					signal.thesis.AppendMeasurements(
						types.SourceCVD, measurements, true,
					)
				}

				signal.status.Store(types.READY)
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers := thesis.MarketTickers(types.SourceCVD)
	trades := thesis.MarketTrades(types.SourceCVD)
	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)
	tradeBatches := make(map[string][]kraken.TradeData)
	symbols := make([]string, 0)
	results := &sync.Map{}
	errorsBySymbol := &sync.Map{}

	for _, row := range tickers {
		signal.observeMidpoint(row)
	}

	for _, row := range trades {
		if !validTrade(row) {
			continue
		}

		if _, exists := tradeBatches[row.Symbol]; !exists {
			symbols = append(symbols, row.Symbol)
		}

		tradeBatches[row.Symbol] = append(tradeBatches[row.Symbol], row)
	}

	for _, symbol := range symbols {
		sort.SliceStable(tradeBatches[symbol], func(leftIndex, rightIndex int) bool {
			left := tradeBatches[symbol][leftIndex]
			right := tradeBatches[symbol][rightIndex]

			if left.Timestamp.Equal(right.Timestamp) {
				return left.TradeID < right.TradeID
			}

			return left.Timestamp.Before(right.Timestamp)
		})
	}

	signal.ensureProcessors()

	group, _ := errgroup.WithContext(signal.ctx)

	for _, symbol := range symbols {
		symbolTrades := tradeBatches[symbol]

		group.Go(func() error {
			symbolMeasurements := make([]*types.Measurement, 0)

			for _, row := range symbolTrades {
				if signal.seenTrade(row) {
					continue
				}

				midpoint, index, exists := signal.midpointAt(row.Symbol, row.Timestamp)

				if !exists {
					continue
				}

				tradeMeasurements, err := signal.measureTrade(row, midpoint.price)

				if err != nil {
					results.Store(symbol, symbolMeasurements)
					errorsBySymbol.Store(symbol, err)
					return nil
				}

				signal.commitTrade(row)
				signal.commitMidpoint(row.Symbol, index)
				symbolMeasurements = append(symbolMeasurements, tradeMeasurements...)
			}

			results.Store(symbol, symbolMeasurements)
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"cvd: parallel measurement failed",
			err,
		))
	}

	errorsBySymbol.Range(func(key, value any) bool {
		err := value.(error)
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"cvd: failed to measure trade",
			err,
		))
		return true
	})

	for _, symbol := range symbols {
		raw, exists := results.Load(symbol)

		if !exists {
			continue
		}

		symbolMeasurements := raw.([]*types.Measurement)
		measurements = append(measurements, symbolMeasurements...)

		if symbol == types.Focus() {
			out = append(out, symbolMeasurements...)
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
		signal.midpoints = &sync.Map{}
	}

	raw, exists := signal.midpoints.Load(row.Symbol)
	observations := make([]midpointObservation, 0)

	if exists {
		observations = raw.([]midpointObservation)
	}

	observation := midpointObservation{at: row.Timestamp, price: (bid + ask) / 2}
	insertAt := len(observations)

	for index, existing := range observations {
		if existing.at.Equal(observation.at) {
			observations[index] = observation
			signal.midpoints.Store(row.Symbol, observations)

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
	signal.midpoints.Store(row.Symbol, observations)
}

func (signal *Signal) midpointAt(
	symbol string,
	at time.Time,
) (midpointObservation, int, bool) {
	if signal.midpoints == nil {
		return midpointObservation{}, 0, false
	}

	raw, exists := signal.midpoints.Load(symbol)

	if !exists {
		return midpointObservation{}, 0, false
	}

	observations := raw.([]midpointObservation)

	for index := len(observations) - 1; index >= 0; index-- {
		if !observations[index].at.After(at) {
			return observations[index], index, true
		}
	}

	return midpointObservation{}, 0, false
}

func (signal *Signal) commitMidpoint(symbol string, index int) {
	raw, exists := signal.midpoints.Load(symbol)

	if !exists {
		return
	}

	observations := raw.([]midpointObservation)

	if index <= 0 || index >= len(observations) {
		return
	}

	signal.midpoints.Store(symbol, observations[index:])
}

func validTrade(row kraken.TradeData) bool {
	price := row.Price.Float64()

	return row.Symbol != "" && !row.Timestamp.IsZero() && price > 0 && row.Qty > 0 &&
		!math.IsNaN(price) && !math.IsInf(price, 0) && !math.IsNaN(row.Qty) &&
		!math.IsInf(row.Qty, 0) && (row.Side == "buy" || row.Side == "sell")
}

func (signal *Signal) seenTrade(row kraken.TradeData) bool {
	if signal.lastTrade == nil {
		return false
	}

	raw, exists := signal.lastTrade.Load(row.Symbol)

	if !exists {
		return false
	}

	previous := raw.(tradeCursor)

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
	if signal.lastTrade == nil {
		signal.lastTrade = &sync.Map{}
	}

	previous := tradeCursor{}
	raw, exists := signal.lastTrade.Load(row.Symbol)

	if exists {
		previous = raw.(tradeCursor)
	}

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
	signal.lastTrade.Store(row.Symbol, previous)
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

	input, _, err := sample.Measure(algorithm.TradeFlowInput{
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

	output, err := flow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	return signal.cvdMeasurements(row, midpoint, output, input.TradeCount), nil
}

/*
cvdMeasurements maps signed-flow output into one source×symbol row whose
Metrics map preserves each reading's unit and normalization.
*/
func (signal *Signal) cvdMeasurements(
	row kraken.TradeData,
	midpoint float64,
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

	measurement := &types.Measurement{
		ID:     uuid.NewString(),
		Source: types.SourceCVD,
		Symbol: row.Symbol,
		At:     row.Timestamp,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricMidpoint, types.SideNone): {
				Raw:  midpoint,
				Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricTradePrice, types.SideNone): {
				Raw:  row.Price.Float64(),
				Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricTradeQuantity, types.SideNone): {
				Raw:  row.Qty,
				Unit: types.UnitBaseCurrency,
			},
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
	metricType types.MetricType,
	raw float64,
	evidenceCount int,
) *float64 {
	switch metricType {
	case types.MetricAbsorption, types.MetricDrive, types.MetricStarvation:
		if evidenceCount < minimumPriceResponseObservations {
			return nil
		}
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
	value := math.Copysign(netFraction, raw)
	_ = evidenceCount

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
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
