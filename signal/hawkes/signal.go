package hawkes

import (
	"context"
	"maps"
	"math"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

const criticalBranch = 1.0

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
	status     atomic.Value
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	processors *sync.Map
	ui         chan []byte
	thesis     *types.Thesis
	semaphore  chan struct{}
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Each
update reconstructs marked-arrival history from the prior Thesis measurement.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		processors: &sync.Map{},
		ui:         ui,
		thesis:     thesis,
		semaphore:  make(chan struct{}, 1),
	}

	signal.status.Store(types.INITIALIZING)
	signal.thesis.Subscribe(types.SourceHawkes, signal.semaphore)
	signal.run()
	signal.status.Store(types.READY)

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
				signal.thesis.AppendMeasurements(
					types.SourceHawkes,
					signal.Measure(signal.thesis), true,
				)
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, _ := signal.measure(thesis)
	return measurements
}

func (signal *Signal) measure(thesis *types.Thesis) ([]*types.Measurement, bool) {
	trades := thesis.MarketTrades(types.SourceHawkes)

	if len(trades) == 0 {
		return nil, false
	}

	previous := make(map[string]*types.Measurement)

	for _, measurement := range utils.Measurements(thesis, types.SourceHawkes) {
		current, found := previous[measurement.Symbol]

		if !found || measurement.At.After(current.At) {
			previous[measurement.Symbol] = measurement
		}
	}

	histories := make(map[string]*arrivalHistory)

	for _, trade := range trades {
		history, found := histories[trade.Symbol]

		if !found {
			history = newArrivalHistory(previous[trade.Symbol])
			histories[trade.Symbol] = history
		}

		if err := history.Append(trade); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"hawkes: invalid market trade",
				err,
			))

			return nil, false
		}
	}

	var (
		collected    sync.Mutex
		measurements = make([]*types.Measurement, 0, len(histories))
		out          = make([]*types.Measurement, 0)
		group, _     = errgroup.WithContext(signal.ctx)
		ready        bool
	)

	measure := func(history *arrivalHistory) func() error {
		return func() error {
			input, found := history.Input()

			if !found {
				return nil
			}

			process, _ := signal.processors.LoadOrStore(
				input.Symbol,
				excitation.NewProcess(),
			)

			outcome, ok, err := process.(*excitation.Process).Measure(input)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"excitation measure failed: "+err.Error(),
					err,
				))
			}

			if !ok {
				return nil
			}

			measurement := signal.measurement(input.Symbol, outcome)
			measurement.Arrivals = history.Arrivals(outcome.ObservedFrom)

			collected.Lock()
			defer collected.Unlock()

			measurements = append(measurements, measurement)
			ready = ready || outcome.Readiness.Intensity

			if input.Symbol == types.Focus() {
				out = append(out, measurement)
			}

			return nil
		}
	}

	for _, history := range histories {
		group.Go(measure(history))
	}

	if err := group.Wait(); err != nil {
		return nil, false
	}

	sort.Slice(measurements, func(left, right int) bool {
		return measurements[left].Symbol < measurements[right].Symbol
	})
	sort.Slice(out, func(left, right int) bool {
		return out[left].Symbol < out[right].Symbol
	})

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements, ready && len(measurements) > 0
}

/*
measurement projects one excitation outcome onto the measurement wire. Fitted
state is written only once the kernel is identified, so a metric that is absent
stays distinguishable from one measured at zero.
*/
func (signal *Signal) measurement(
	symbol string,
	outcome excitation.Outcome,
) *types.Measurement {
	fit := outcome.Fit
	events := float64(outcome.EventCount)
	immigrant := fit.MuX + fit.MuY
	horizon := outcome.Horizon

	if !outcome.ObservedFrom.IsZero() && outcome.At.After(outcome.ObservedFrom) {
		horizon = outcome.At.Sub(outcome.ObservedFrom)
	}

	// Before a fit the only shared scale is the total marked rate the two sides
	// were drawn from; after one, each side has its own immigrant baseline.
	marked := outcome.BuyArrivalRate + outcome.SellArrivalRate
	buyBaseline, sellBaseline := marked, marked

	if outcome.Readiness.HawkesFit {
		buyBaseline, sellBaseline = fit.MuX, fit.MuY
	}

	measurement := &types.Measurement{
		Source:       types.SourceHawkes,
		Symbol:       symbol,
		At:           outcome.At,
		ObservedFrom: outcome.ObservedFrom,
		Horizon:      horizon,
		Maturity:     outcome.Maturity,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricEventCount, types.SideNone): {
				Raw:        events,
				Normalized: normalizedShare(events, float64(outcome.MinimumFitEvents)),
				Unit:       types.UnitCount,
			},
			types.MetricKey(types.MetricEventCount, types.SideBuy): {
				Raw:        float64(outcome.BuyEventCount),
				Normalized: normalizedShare(float64(outcome.BuyEventCount), events),
				Unit:       types.UnitCount,
			},
			types.MetricKey(types.MetricEventCount, types.SideSell): {
				Raw:        float64(outcome.SellEventCount),
				Normalized: normalizedShare(float64(outcome.SellEventCount), events),
				Unit:       types.UnitCount,
			},
			types.MetricKey(types.MetricArrivalRate, types.SideBuy): {
				Raw:        outcome.BuyArrivalRate,
				Normalized: normalizedRate(outcome.BuyArrivalRate, buyBaseline, outcome),
				Unit:       types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricArrivalRate, types.SideSell): {
				Raw:        outcome.SellArrivalRate,
				Normalized: normalizedRate(outcome.SellArrivalRate, sellBaseline, outcome),
				Unit:       types.UnitEventsPerSecond,
			},
		},
	}

	if !outcome.Readiness.HawkesFit {
		return measurement
	}

	// A nonnegative kernel cannot hold a conditional rate below its own
	// immigrant baseline, and A/beta is the offspring one parent arrival is
	// expected to produce, so an amplitude is scaled by the decay consuming it.
	fitted := map[string]types.MetricSample{
		types.MetricKey(types.MetricConditionalIntensity, types.SideBuy): {
			Raw:        fit.IntensityX,
			Normalized: normalizedExcess(fit.IntensityX, fit.MuX),
			Unit:       types.UnitEventsPerSecond,
		},
		types.MetricKey(types.MetricConditionalIntensity, types.SideSell): {
			Raw:        fit.IntensityY,
			Normalized: normalizedExcess(fit.IntensityY, fit.MuY),
			Unit:       types.UnitEventsPerSecond,
		},
		types.MetricKey(types.MetricBaselineIntensity, types.SideBuy): {
			Raw:        fit.MuX,
			Normalized: normalizedShare(fit.MuX, immigrant),
			Unit:       types.UnitEventsPerSecond,
		},
		types.MetricKey(types.MetricBaselineIntensity, types.SideSell): {
			Raw:        fit.MuY,
			Normalized: normalizedShare(fit.MuY, immigrant),
			Unit:       types.UnitEventsPerSecond,
		},
		types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy): {
			Raw:        fit.AlphaXX,
			Normalized: normalizedShare(fit.AlphaXX, fit.Beta),
			Unit:       types.UnitEventsPerSecond,
		},
		types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToBuy): {
			Raw:        fit.AlphaXY,
			Normalized: normalizedShare(fit.AlphaXY, fit.Beta),
			Unit:       types.UnitEventsPerSecond,
		},
		types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToSell): {
			Raw:        fit.AlphaYX,
			Normalized: normalizedShare(fit.AlphaYX, fit.Beta),
			Unit:       types.UnitEventsPerSecond,
		},
		types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToSell): {
			Raw:        fit.AlphaYY,
			Normalized: normalizedShare(fit.AlphaYY, fit.Beta),
			Unit:       types.UnitEventsPerSecond,
		},
		types.MetricKey(types.MetricDecayRate, types.SideNone): {
			Raw:        fit.Beta,
			Normalized: normalizedShare(fit.Beta, immigrant),
			Unit:       types.UnitInverseSecond,
		},
		types.MetricKey(types.MetricKernelMemory, types.SideNone): {
			Raw:        1 / fit.Beta,
			Normalized: normalizedShare(1/fit.Beta, horizon.Seconds()),
			Unit:       types.UnitSecond,
		},
		types.MetricKey(types.MetricSpectralRadius, types.SideNone): {
			Raw:        fit.SpectralRadius,
			Normalized: normalizedBranching(fit.SpectralRadius),
			Unit:       types.UnitDimensionless,
		},
		types.MetricKey(types.MetricHawkesPoissonDelta, types.SideNone): {
			Raw:        outcome.HawkesPoissonLogLikelihoodDelta,
			Normalized: normalizedShare(outcome.HawkesPoissonLogLikelihoodDelta, events),
			Unit:       types.UnitNat,
		},
		types.MetricKey(types.MetricCrossSelfDelta, types.SideNone): {
			Raw:        outcome.CrossSelfLogLikelihoodDelta,
			Normalized: normalizedShare(outcome.CrossSelfLogLikelihoodDelta, events),
			Unit:       types.UnitNat,
		},
		types.MetricKey(types.MetricImmediateOffspring, types.SideBuy): {
			Raw:        outcome.ImmediateBuyOffspring,
			Normalized: normalizedExpectation(outcome.ImmediateBuyOffspring),
			Unit:       types.UnitDimensionless,
		},
		types.MetricKey(types.MetricImmediateOffspring, types.SideSell): {
			Raw:        outcome.ImmediateSellOffspring,
			Normalized: normalizedExpectation(outcome.ImmediateSellOffspring),
			Unit:       types.UnitDimensionless,
		},
		types.MetricKey(types.MetricTotalDescendants, types.SideBuy): {
			Raw:        outcome.TotalBuyDescendants,
			Normalized: normalizedExpectation(outcome.TotalBuyDescendants),
			Unit:       types.UnitDimensionless,
		},
		types.MetricKey(types.MetricTotalDescendants, types.SideSell): {
			Raw:        outcome.TotalSellDescendants,
			Normalized: normalizedExpectation(outcome.TotalSellDescendants),
			Unit:       types.UnitDimensionless,
		},
	}

	maps.Copy(measurement.Metrics, fitted)
	return measurement
}

/*
normalizedShare reports a reading as a fraction of the positive reference scale
that makes it comparable across symbols.
*/
func normalizedShare(raw, reference float64) *float64 {
	if reference <= 0 || math.IsNaN(raw) || math.IsInf(raw, 0) ||
		math.IsNaN(reference) || math.IsInf(reference, 0) {
		return nil
	}

	value := raw / reference

	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}

	return &value
}

/*
normalizedRate scales an empirical arrival rate against whichever baseline the
estimator has established. Before a fit that is the total marked rate the two
sides were drawn from, so the reading is this mark's share of all arrivals.
After a fit it is this mark's own immigrant intensity, so the reading is the
excitation carried above that baseline. Below intensity readiness there is no
rate to compare against at all.
*/
func normalizedRate(raw, baseline float64, outcome excitation.Outcome) *float64 {
	if !outcome.Readiness.Intensity || outcome.EventCount <= 1 || raw < 0 {
		return nil
	}

	if !outcome.Readiness.HawkesFit {
		return normalizedShare(raw, baseline)
	}

	return normalizedExcess(raw, baseline)
}

/*
normalizedExcess reports a rate in baselines above its reference, which the
nonnegative-kernel Hawkes contract requires to be at or above that reference.
*/
func normalizedExcess(raw, baseline float64) *float64 {
	if raw < baseline {
		return nil
	}

	return normalizedShare(raw-baseline, baseline)
}

/*
normalizedBranching publishes a branching ratio only while the process is
stationary. At or above one the expected cascade size diverges and the number
stops describing anything observable.
*/
func normalizedBranching(raw float64) *float64 {
	if raw < 0 || raw >= criticalBranch || math.IsNaN(raw) || math.IsInf(raw, 0) {
		return nil
	}

	value := raw

	return &value
}

/*
normalizedExpectation accepts an offspring or descendant count, which a fitted
branching structure cannot make negative.
*/
func normalizedExpectation(raw float64) *float64 {
	if raw < 0 || math.IsNaN(raw) || math.IsInf(raw, 0) {
		return nil
	}

	value := raw

	return &value
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
