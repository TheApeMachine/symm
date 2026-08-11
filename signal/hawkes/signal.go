package hawkes

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures the buy/sell trade-arrival process as

	λ(t) = μ + Σ A exp(-β(t-ti)).

It emits empirical rates before the model is identifiable, then fitted μ, λ,
A, β, spectral stability, offspring expectations, and restricted likelihood
comparisons. These are statistical measurements rather than market regimes;
forecast readiness remains false until residual and out-of-sample validation
exists.
*/
type Signal struct {
	status    atomic.Value
	ctx       context.Context
	cancel    context.CancelFunc
	api       *websocket.API
	process   *excitation.Process
	sample    *excitation.Sample
	ui        chan []byte
	thesis    *types.Thesis
	semaphore chan struct{}
	lastTrade *sync.Map
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

/*
NewSignal constructs the excitation pipeline. Nomagique owns the symbol-local
arrival windows and fitted parameter epochs.
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
		process:   excitation.NewProcess(),
		sample:    excitation.NewSample(),
		ui:        ui,
		thesis:    thesis,
		semaphore: make(chan struct{}, 1),
		lastTrade: &sync.Map{},
	}

	signal.status.Store(types.INITIALIZING)
	signal.thesis.Subscribe(types.SourceHawkes, signal.semaphore, &signal.status)
	signal.status.Store(types.READY)
	signal.run()

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceHawkes)
}

func (signal *Signal) Status() types.Status {
	return signal.status.Load().(types.Status)
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case <-signal.semaphore:
				measurements, ready := signal.Measure(signal.thesis)

				if len(measurements) > 0 {
					signal.thesis.AppendMeasurements(
						types.SourceHawkes, measurements, ready,
					)
				}

				signal.thesis.StampAll(types.SourceHawkes)
				signal.status.Store(types.READY)
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) ([]*types.Measurement, bool) {
	trades := thesis.MarketTrades(types.SourceHawkes)

	if len(trades) == 0 {
		return nil, false
	}

	measurements := make([]*types.Measurement, 0)
	var focused *types.Measurement
	ready := false

	for index, trade := range trades {
		if signal.seenTrade(trade) {
			continue
		}

		input, sampled, err := signal.sample.MeasureArrival(excitation.TradeInput{
			Symbol:    trade.Symbol,
			Side:      trade.Side,
			Timestamp: trade.Timestamp,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"excitation sample failed: "+err.Error(),
				err,
			))

			continue
		}

		signal.commitTrade(trade)

		if !sampled {
			continue
		}

		outcome, measured, err := signal.process.Measure(input)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"excitation measure failed: "+err.Error(),
				err,
			))

			continue
		}

		if !measured {
			continue
		}

		latest := true

		for _, pending := range trades[index+1:] {
			if pending.Symbol == trade.Symbol {
				latest = false
				break
			}
		}

		if !latest {
			continue
		}

		branching := outcome.Fit.Params().BranchingMatrix()
		var buyToBuy *float64
		var sellToBuy *float64
		var buyToSell *float64
		var sellToSell *float64
		var spectralRadius *float64

		if outcome.Readiness.HawkesFit {
			buyToBuy = &branching[0][0]
			sellToBuy = &branching[0][1]
			buyToSell = &branching[1][0]
			sellToSell = &branching[1][1]
			spectralRadius = &outcome.Fit.SpectralRadius
		}

		measurement := &types.Measurement{
			ID:           uuid.NewString(),
			Source:       types.SourceHawkes,
			Symbol:       input.Symbol,
			At:           outcome.At,
			ObservedFrom: outcome.ObservedFrom,
			Horizon:      outcome.Horizon,
			Maturity:     outcome.Maturity,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricEventCount, types.SideNone): {
					Raw:  float64(outcome.EventCount),
					Unit: types.UnitCount,
				},
				types.MetricKey(types.MetricEventCount, types.SideBuy): {
					Raw:  float64(outcome.BuyEventCount),
					Unit: types.UnitCount,
				},
				types.MetricKey(types.MetricEventCount, types.SideSell): {
					Raw:  float64(outcome.SellEventCount),
					Unit: types.UnitCount,
				},
				types.MetricKey(types.MetricArrivalRate, types.SideBuy): {
					Raw:  outcome.BuyArrivalRate,
					Unit: types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricArrivalRate, types.SideSell): {
					Raw:  outcome.SellArrivalRate,
					Unit: types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricConditionalIntensity, types.SideBuy): {
					Raw:  outcome.Fit.IntensityX,
					Unit: types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricConditionalIntensity, types.SideSell): {
					Raw:  outcome.Fit.IntensityY,
					Unit: types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricBaselineIntensity, types.SideBuy): {
					Raw:  outcome.Fit.MuX,
					Unit: types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricBaselineIntensity, types.SideSell): {
					Raw:  outcome.Fit.MuY,
					Unit: types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy): {
					Raw:        outcome.Fit.AlphaXX,
					Normalized: buyToBuy,
					Unit:       types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToBuy): {
					Raw:        outcome.Fit.AlphaXY,
					Normalized: sellToBuy,
					Unit:       types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToSell): {
					Raw:        outcome.Fit.AlphaYX,
					Normalized: buyToSell,
					Unit:       types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToSell): {
					Raw:        outcome.Fit.AlphaYY,
					Normalized: sellToSell,
					Unit:       types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricDecayRate, types.SideNone): {
					Raw:  outcome.Fit.Beta,
					Unit: types.UnitInverseSecond,
				},
				types.MetricKey(types.MetricKernelMemory, types.SideNone): {
					Raw:  outcome.Fit.Runway().Seconds(),
					Unit: types.UnitSecond,
				},
				types.MetricKey(types.MetricSpectralRadius, types.SideNone): {
					Raw:        outcome.Fit.SpectralRadius,
					Normalized: spectralRadius,
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricHawkesPoissonDelta, types.SideNone): {
					Raw:  outcome.HawkesPoissonLogLikelihoodDelta,
					Unit: types.UnitNat,
				},
				types.MetricKey(types.MetricCrossSelfDelta, types.SideNone): {
					Raw:  outcome.CrossSelfLogLikelihoodDelta,
					Unit: types.UnitNat,
				},
				types.MetricKey(types.MetricImmediateOffspring, types.SideBuy): {
					Raw:  outcome.ImmediateBuyOffspring,
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricImmediateOffspring, types.SideSell): {
					Raw:  outcome.ImmediateSellOffspring,
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricTotalDescendants, types.SideBuy): {
					Raw:  outcome.TotalBuyDescendants,
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricTotalDescendants, types.SideSell): {
					Raw:  outcome.TotalSellDescendants,
					Unit: types.UnitDimensionless,
				},
			},
		}
		snr, snrReady := types.MeasurementSignalNoiseRatio(
			types.SourceHawkes,
			measurement.Metrics,
		)
		snrSample := types.MetricSample{
			Raw:  snr,
			Unit: types.UnitDimensionless,
		}

		if snrReady {
			snrSample.Normalized = &snr
		}

		measurement.PutMetric(types.MetricSNR, types.SideNone, snrSample)

		measurements = append(measurements, measurement)
		ready = ready || outcome.Readiness.Intensity

		if measurement.Symbol == types.Focus() {
			focused = measurement
		}
	}

	if focused == nil || signal.ui == nil {
		return measurements, ready
	}

	payload, err := sonic.Marshal(struct {
		Measurements []*types.Measurement `json:"measurements"`
	}{Measurements: []*types.Measurement{focused}})

	if err != nil {
		panic(errnie.Error(errnie.Err(
			errnie.Validation,
			"failed to marshal Hawkes measurement: "+err.Error(),
			err,
		)))
	}

	select {
	case signal.ui <- payload:
	default:
	}

	return measurements, ready
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
		previous = tradeCursor{at: row.Timestamp, ids: make(map[int64]struct{})}
	}

	if previous.ids == nil {
		previous.ids = make(map[int64]struct{})
	}

	previous.ids[row.TradeID] = struct{}{}
	signal.lastTrade.Store(row.Symbol, previous)
}

/*
Close releases the receiver's owned resources.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
