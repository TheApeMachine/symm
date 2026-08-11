package hawkes

import (
	"context"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/probability"
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
		buyHypothesis := outcome.BuyArrivalRate
		sellHypothesis := outcome.SellArrivalRate

		if outcome.Readiness.HawkesFit {
			buyHypothesis = outcome.ImmediateBuyOffspring
			sellHypothesis = outcome.ImmediateSellOffspring
		}

		// Self- and cross-excitation are complementary paths within each parent
		// side. The column-summed buy and sell offspring totals are the competing
		// fitted hypotheses; directly observed arrival rates carry that role before
		// the fit is identifiable.
		snr, err := probability.SignalNoiseRatio([]float64{
			buyHypothesis,
			sellHypothesis,
		})

		if err != nil {
			panic(err)
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
				types.MetricKey(types.MetricSNR, types.SideNone): {
					Raw:        snr,
					Normalized: &snr,
					Unit:       types.UnitDimensionless,
				},
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
					Normalized: &branching[0][0],
					Unit:       types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToBuy): {
					Raw:        outcome.Fit.AlphaXY,
					Normalized: &branching[0][1],
					Unit:       types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToSell): {
					Raw:        outcome.Fit.AlphaYX,
					Normalized: &branching[1][0],
					Unit:       types.UnitEventsPerSecond,
				},
				types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToSell): {
					Raw:        outcome.Fit.AlphaYY,
					Normalized: &branching[1][1],
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
					Normalized: &outcome.Fit.SpectralRadius,
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

/*
Close releases the receiver's owned resources.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
