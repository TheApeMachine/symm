package hawkes

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/types"
)

const MaxArrivalSamples = 64

type arrivalSample struct {
	timestamp time.Time
	atSec     float64
	mark      float64
}

/*
State maintains the online arrival history and fitted Hawkes parameters for one stream.
*/
type State struct {
	samples        []arrivalSample
	lastTimestamp  time.Time
	hasLast        bool
	model          bivariateFit
	modelReady     bool
	selfOnlyModel  bivariateFit
	selfOnlyReady  bool
	eventsSinceFit int
	modelSupport   float64

	readings    []types.Reading
	rejected    bool
	from        time.Time
	snr         float64
	hasSNR      bool
	measurement *data.Measurement[float64]
}

/*
Bivariate is the causal, online bivariate Hawkes arrival process model node.
It maintains bounded arrival history and refits MLE parameters as data warrants.
When configured with a Key or KeyStore slot, state is isolated per key without
duplicating the pipeline graph.
*/
type Bivariate struct {
	Mark  types.Node
	Clock types.Node
	Store *store.KeyStore
	Key   func() string

	states map[string]*State
	single State
	active *State
}

/*
NewBivariate constructs a new Bivariate Hawkes process model node.
*/
func NewBivariate(clock types.Node) *Bivariate {
	return &Bivariate{
		Clock:  clock,
		states: make(map[string]*State),
	}
}

/*
NewBivariateWithKey constructs a Bivariate Hawkes node configured with a KeyStore or Key selector.
*/
func NewBivariateWithKey(
	clock types.Node,
	mark types.Node,
	keyStore *store.KeyStore,
	key func() string,
) *Bivariate {
	return &Bivariate{
		Clock:  clock,
		Mark:   mark,
		Store:  keyStore,
		Key:    key,
		states: make(map[string]*State),
	}
}

/*
Step advances the Hawkes arrival process with one observation.
*/
func (bivariate *Bivariate) Step(carrier types.Scalar) types.Scalar {
	state := bivariate.resolveState()
	bivariate.active = state

	state.rejected = false
	state.hasSNR = false
	state.readings = nil

	mark := float64(carrier)

	if bivariate.Mark != nil {
		mark = float64(bivariate.Mark.Step(carrier))
	}

	key := bivariate.activeKey()

	if mark == 0 {
		state.rejected = true

		err := errors.New("hawkes: a finite non-zero mark is required")
		errnie.Error(errnie.Err(errnie.Validation, "hawkes: invalid mark", err))

		state.measurement = &data.Measurement[float64]{
			ID:     fmt.Sprintf("%s:hawkes:%s", key, time.Now().Format(time.RFC3339Nano)),
			Label:  key,
			Source: "hawkes",
			Err:    err,
		}

		return carrier
	}

	var at time.Time

	if timeSource, ok := bivariate.Clock.(interface{ Time() time.Time }); ok {
		at = timeSource.Time()
	}

	if at.IsZero() {
		at = time.Now()
	}

	if state.hasLast {
		if at.Before(state.lastTimestamp) {
			state.rejected = true

			err := errors.New("hawkes: regressing event time")
			errnie.Error(errnie.Err(errnie.Validation, "hawkes: regressing event time", err))

			state.measurement = &data.Measurement[float64]{
				ID:     fmt.Sprintf("%s:hawkes:%s", key, at.Format(time.RFC3339Nano)),
				Label:  key,
				Source: "hawkes",
				At:     at,
				Err:    err,
			}

			return carrier
		}
	}

	state.lastTimestamp = at
	state.hasLast = true

	buyArrivals := make([]float64, 0, len(state.samples))
	sellArrivals := make([]float64, 0, len(state.samples))

	for _, sample := range state.samples {
		if sample.mark > 0 {
			buyArrivals = append(buyArrivals, sample.atSec)
			continue
		}

		sellArrivals = append(sellArrivals, sample.atSec)
	}

	countBuy := float64(len(buyArrivals))
	countSell := float64(len(sellArrivals))

	if mark > 0 {
		countBuy++
	}

	if mark <= 0 {
		countSell++
	}

	count := countBuy + countSell

	from := at

	if len(state.samples) > 0 {
		from = state.samples[0].timestamp
	}

	state.from = from

	atSec := float64(at.UnixNano()) * 1e-9
	fromSec := float64(from.UnixNano()) * 1e-9
	span := atSec - fromSec

	id := fmt.Sprintf("%s:hawkes:%s", key, at.Format(time.RFC3339Nano))
	measurement := data.NewMeasurement[float64](id, key, "hawkes", at, from)

	readings := make([]types.Reading, 0, 48)

	addMetric := func(label string, val float64, unit data.Unit, timescale data.Timescale) {
		measurement.PutMetric(data.Metric[float64]{
			Label:     label,
			Raw:       val,
			Unit:      unit,
			Timescale: timescale,
		})

		readings = append(readings, types.Reading{
			Label:     label,
			Value:     types.Scalar(val),
			Unit:      string(unit),
			Timescale: string(timescale),
			Defined:   true,
		})
	}

	addMetric("event_count", count, data.UnitCount, data.TimescaleInstantaneous)
	addMetric("event_count:buy", countBuy, data.UnitCount, data.TimescaleInstantaneous)
	addMetric("event_count:sell", countSell, data.UnitCount, data.TimescaleInstantaneous)
	addMetric("event_fraction:buy", countBuy/count, data.UnitDimensionless, data.TimescaleInstantaneous)
	addMetric("event_fraction:sell", countSell/count, data.UnitDimensionless, data.TimescaleInstantaneous)

	if span > 0 {
		rateBuy := countBuy / span
		rateSell := countSell / span

		addMetric("arrival_rate:buy", rateBuy, data.UnitPerSecond, data.TimescalePerSecond)
		addMetric("arrival_rate:sell", rateSell, data.UnitPerSecond, data.TimescalePerSecond)
		addMetric("arrival_rate", rateBuy+rateSell, data.UnitPerSecond, data.TimescalePerSecond)
	}

	if state.modelReady {
		state.evaluateModel(measurement, &readings, buyArrivals, sellArrivals, atSec, span, mark)
	}

	state.readings = readings

	metadata := map[string]float64{
		data.MetadataSupport: state.Support(),
	}

	if state.hasSNR {
		metadata[data.MetadataDivergence] = float64(state.Divergence())
		metadata[data.MetadataNoiseVariance] = float64(state.NoiseVariance())
	}

	if len(metadata) > 0 {
		measurement.Metadata = metadata
	}

	measurement.Finalize()
	state.measurement = measurement

	if len(state.samples) >= MaxArrivalSamples {
		state.samples = state.samples[1:]
	}

	state.samples = append(state.samples, arrivalSample{
		timestamp: at,
		atSec:     atSec,
		mark:      mark,
	})

	state.tryRefit(at, atSec)

	return carrier
}

func (bivariate *Bivariate) resolveState() *State {
	key := bivariate.activeKey()

	if key != "" {
		if bivariate.states == nil {
			bivariate.states = make(map[string]*State)
		}

		st, found := bivariate.states[key]

		if !found {
			st = &State{
				samples: make([]arrivalSample, 0, MaxArrivalSamples),
			}
			bivariate.states[key] = st
		}

		return st
	}

	if bivariate.single.samples == nil {
		bivariate.single.samples = make([]arrivalSample, 0, MaxArrivalSamples)
	}

	return &bivariate.single
}

func (bivariate *Bivariate) activeKey() string {
	if bivariate.Key != nil {
		return bivariate.Key()
	}

	if bivariate.Store != nil && bivariate.Store.Key != nil {
		return bivariate.Store.Key()
	}

	return ""
}

func (state *State) evaluateModel(
	measurement *data.Measurement[float64],
	readings *[]types.Reading,
	buyArrivals []float64,
	sellArrivals []float64,
	atSec float64,
	span float64,
	mark float64,
) {
	muX := state.model.muX
	muY := state.model.muY
	alphaXX := state.model.alphaXX
	alphaXY := state.model.alphaXY
	alphaYX := state.model.alphaYX
	alphaYY := state.model.alphaYY
	beta := state.model.beta

	lambdaBuy := intensityAt(buyArrivals, sellArrivals, atSec, muX, alphaXX, alphaXY, beta)
	lambdaSell := intensityAt(buyArrivals, sellArrivals, atSec, muY, alphaYX, alphaYY, beta)

	excessBuy := lambdaBuy - muX
	excessSell := lambdaSell - muY

	addMetric := func(label string, val float64, unit data.Unit, timescale data.Timescale) {
		measurement.PutMetric(data.Metric[float64]{
			Label:     label,
			Raw:       val,
			Unit:      unit,
			Timescale: timescale,
		})

		*readings = append(*readings, types.Reading{
			Label:     label,
			Value:     types.Scalar(val),
			Unit:      string(unit),
			Timescale: string(timescale),
			Defined:   true,
		})
	}

	addMetric("conditional_intensity:buy", lambdaBuy, data.UnitPerSecond, data.TimescalePerSecond)
	addMetric("conditional_intensity:sell", lambdaSell, data.UnitPerSecond, data.TimescalePerSecond)
	addMetric("conditional_intensity", lambdaBuy+lambdaSell, data.UnitPerSecond, data.TimescalePerSecond)
	addMetric("background_rate:buy", muX, data.UnitPerSecond, data.TimescalePerSecond)
	addMetric("background_rate:sell", muY, data.UnitPerSecond, data.TimescalePerSecond)
	addMetric("background_rate", muX+muY, data.UnitPerSecond, data.TimescalePerSecond)
	addMetric("excitation_intensity:buy", excessBuy, data.UnitPerSecond, data.TimescalePerSecond)
	addMetric("excitation_intensity:sell", excessSell, data.UnitPerSecond, data.TimescalePerSecond)

	if lambdaBuy > 0 {
		addMetric("excitation_fraction:buy", excessBuy/lambdaBuy, data.UnitDimensionless, data.TimescaleInstantaneous)
	}

	if lambdaSell > 0 {
		addMetric("excitation_fraction:sell", excessSell/lambdaSell, data.UnitDimensionless, data.TimescaleInstantaneous)
	}

	addMetric("excitation_amplitude:buy_from_buy", alphaXX, data.UnitDimensionless, data.TimescaleInstantaneous)
	addMetric("excitation_amplitude:buy_from_sell", alphaXY, data.UnitDimensionless, data.TimescaleInstantaneous)
	addMetric("excitation_amplitude:sell_from_buy", alphaYX, data.UnitDimensionless, data.TimescaleInstantaneous)
	addMetric("excitation_amplitude:sell_from_sell", alphaYY, data.UnitDimensionless, data.TimescaleInstantaneous)

	if beta > 0 {
		timescale := 1.0 / beta

		addMetric("excitation_decay", beta, data.UnitPerSecond, data.TimescalePerSecond)
		addMetric("excitation_decay:buy_from_buy", beta, data.UnitPerSecond, data.TimescalePerSecond)
		addMetric("excitation_decay:buy_from_sell", beta, data.UnitPerSecond, data.TimescalePerSecond)
		addMetric("excitation_decay:sell_from_buy", beta, data.UnitPerSecond, data.TimescalePerSecond)
		addMetric("excitation_decay:sell_from_sell", beta, data.UnitPerSecond, data.TimescalePerSecond)
		addMetric("excitation_timescale", timescale, data.UnitSecond, data.TimescaleInstantaneous)
		addMetric("excitation_timescale:buy_from_buy", timescale, data.UnitSecond, data.TimescaleInstantaneous)
		addMetric("excitation_timescale:buy_from_sell", timescale, data.UnitSecond, data.TimescaleInstantaneous)
		addMetric("excitation_timescale:sell_from_buy", timescale, data.UnitSecond, data.TimescaleInstantaneous)
		addMetric("excitation_timescale:sell_from_sell", timescale, data.UnitSecond, data.TimescaleInstantaneous)
	}

	matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	addMetric("offspring:buy_from_buy", matrix[0][0], data.UnitDimensionless, data.TimescaleInstantaneous)
	addMetric("offspring:buy_from_sell", matrix[0][1], data.UnitDimensionless, data.TimescaleInstantaneous)
	addMetric("offspring:sell_from_buy", matrix[1][0], data.UnitDimensionless, data.TimescaleInstantaneous)
	addMetric("offspring:sell_from_sell", matrix[1][1], data.UnitDimensionless, data.TimescaleInstantaneous)
	addMetric("branching_spectral_radius", spectralRadius(matrix), data.UnitDimensionless, data.TimescaleInstantaneous)

	buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if hasDesc {
		addMetric("expected_descendants_from_buy", buyParent, data.UnitCount, data.TimescaleInstantaneous)
		addMetric("expected_descendants_from_sell", sellParent, data.UnitCount, data.TimescaleInstantaneous)
	}

	streamWindow := currentWindowStream(buyArrivals, sellArrivals, atSec, mark)
	hawkesLL := state.model.logLikelihood(streamWindow, atSec)
	poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
	poissonLL := poisson.logLikelihood(streamWindow, atSec)

	if len(streamWindow.marked) > 0 {
		markedCount := float64(len(streamWindow.marked))

		addMetric("log_likelihood:hawkes", hawkesLL, data.UnitNat, data.TimescaleInstantaneous)
		addMetric("log_likelihood:poisson", poissonLL, data.UnitNat, data.TimescaleInstantaneous)
		addMetric("log_likelihood_per_event:hawkes", hawkesLL/markedCount, data.UnitNat, data.TimescaleInstantaneous)
		addMetric("log_likelihood_gain_vs_poisson", hawkesLL-poissonLL, data.UnitNat, data.TimescaleInstantaneous)
		addMetric("log_likelihood_gain_per_event_vs_poisson", (hawkesLL-poissonLL)/markedCount, data.UnitNat, data.TimescaleInstantaneous)

		if state.selfOnlyReady {
			selfLL := state.selfOnlyModel.logLikelihood(streamWindow, atSec)

			addMetric("log_likelihood:self_only", selfLL, data.UnitNat, data.TimescaleInstantaneous)
			addMetric("log_likelihood_gain_vs_self_only", hawkesLL-selfLL, data.UnitNat, data.TimescaleInstantaneous)
			addMetric("log_likelihood_gain_per_event_vs_self_only", (hawkesLL-selfLL)/markedCount, data.UnitNat, data.TimescaleInstantaneous)
		}
	}

	streamPrior := newArrivalStream(buyArrivals, sellArrivals)
	spanPrior := streamPrior.span(atSec)

	if spanPrior > 0 {
		buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
		compBuy := muX*spanPrior + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
		compSell := muY*spanPrior + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

		priorCountBuy := float64(len(buyArrivals))
		priorCountSell := float64(len(sellArrivals))
		innoBuy := priorCountBuy - compBuy
		innoSell := priorCountSell - compSell

		addMetric("compensator:buy", compBuy, data.UnitCount, data.TimescaleInstantaneous)
		addMetric("compensator:sell", compSell, data.UnitCount, data.TimescaleInstantaneous)
		addMetric("count_innovation:buy", innoBuy, data.UnitCount, data.TimescaleInstantaneous)
		addMetric("count_innovation:sell", innoSell, data.UnitCount, data.TimescaleInstantaneous)

		if compBuy > 0 {
			addMetric("standardized_innovation:buy", innoBuy/math.Sqrt(compBuy), data.UnitDimensionless, data.TimescaleInstantaneous)
		}

		if compSell > 0 {
			addMetric("standardized_innovation:sell", innoSell/math.Sqrt(compSell), data.UnitDimensionless, data.TimescaleInstantaneous)
		}

		// excitation_share is the excitation's share of the integrated
		// intensity over the whole observation span, where excitation_fraction
		// above is that share at this one instant. The two answer different
		// questions: on a bursty stream the instantaneous form samples a
		// decaying exponential at whatever moment a frame happens to land, so
		// it reads near zero between bursts even when the fit has found strong
		// clustering. The integrated form carries the clustering the fit
		// actually measured, and is what a consumer ranking self-excitation
		// across symbols must read.
		excessBuyMass := compBuy - muX*spanPrior
		excessSellMass := compSell - muY*spanPrior

		addMetric("excitation_mass:buy", excessBuyMass, data.UnitCount, data.TimescaleInstantaneous)
		addMetric("excitation_mass:sell", excessSellMass, data.UnitCount, data.TimescaleInstantaneous)

		if compBuy > 0 {
			addMetric("excitation_share:buy", excessBuyMass/compBuy, data.UnitDimensionless, data.TimescaleInstantaneous)
		}

		if compSell > 0 {
			addMetric("excitation_share:sell", excessSellMass/compSell, data.UnitDimensionless, data.TimescaleInstantaneous)
		}

		if compTotal := compBuy + compSell; compTotal > 0 {
			addMetric(
				"excitation_share",
				(excessBuyMass+excessSellMass)/compTotal,
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
			)
		}

		snrSum := 0.0
		snrSides := 0

		if compBuy > 0 {
			snrSum += (excessBuyMass * excessBuyMass) / compBuy
			snrSides++
		}

		if compSell > 0 {
			snrSum += (excessSellMass * excessSellMass) / compSell
			snrSides++
		}

		if snrSides > 0 {
			snr := snrSum / float64(snrSides)
			state.snr = snr
			state.hasSNR = true

			addMetric("snr", snr, data.UnitDimensionless, data.TimescaleInstantaneous)
		}
	}
}

func (state *State) tryRefit(at time.Time, atSec float64) {
	if len(state.samples) >= 2 {
		if state.samples[len(state.samples)-1].timestamp.Equal(state.samples[len(state.samples)-2].timestamp) {
			return
		}
	}

	buyArrivals := make([]float64, 0, len(state.samples))
	sellArrivals := make([]float64, 0, len(state.samples))

	for _, sample := range state.samples {
		if sample.mark > 0 {
			buyArrivals = append(buyArrivals, sample.atSec)
			continue
		}

		sellArrivals = append(sellArrivals, sample.atSec)
	}

	stream := newArrivalStream(buyArrivals, sellArrivals)
	context, ok := newFitContext(stream, atSec)

	if !ok || !context.enoughEvents(stream) {
		return
	}

	state.eventsSinceFit++

	if state.modelReady && state.eventsSinceFit < context.minFitEvents {
		return
	}

	prior := bivariateFit{}

	if state.modelReady {
		prior = state.model
	}

	estimator := newBivariateEstimator(prior)
	fitted := estimator.fit(stream, atSec)

	if !fitted.valid() {
		return
	}

	state.model = fitted
	state.modelReady = true
	state.modelSupport = float64(context.totalEvents)
	state.eventsSinceFit = 0

	selfOnly := estimator.fitSelfOnly(stream, atSec)

	if selfOnly.valid() {
		state.selfOnlyModel = selfOnly
		state.selfOnlyReady = true
	}
}

func currentWindowStream(buy, sell []float64, horizonSec, mark float64) arrivalStream {
	if mark > 0 {
		return newArrivalStream(append(sortedCopy(buy), horizonSec), sortedCopy(sell))
	}

	return newArrivalStream(sortedCopy(buy), append(sortedCopy(sell), horizonSec))
}

// Measurement returns the Measurement published by the most recent Step.
func (bivariate *Bivariate) Measurement() *data.Measurement[float64] {
	if bivariate.active == nil {
		return nil
	}

	return bivariate.active.measurement
}

// Readings returns the set of readings published by the most recent Step.
func (bivariate *Bivariate) Readings() []types.Reading {
	if bivariate.active == nil {
		return nil
	}

	return bivariate.active.readings
}

// Rejected reports whether the most recent Step was invalid.
func (bivariate *Bivariate) Rejected() bool {
	if bivariate.active == nil {
		return false
	}

	return bivariate.active.rejected
}

// From returns the origin timestamp of the current observation window.
func (bivariate *Bivariate) From() time.Time {
	if bivariate.active == nil {
		return time.Time{}
	}

	return bivariate.active.from
}

// Support reports effective model support for maturity derivation.
func (bivariate *Bivariate) Support() float64 {
	if bivariate.active == nil {
		return 0
	}

	return bivariate.active.Support()
}

// Divergence returns the signal departure for SNR derivation.
func (bivariate *Bivariate) Divergence() types.Scalar {
	if bivariate.active == nil {
		return 0
	}

	return bivariate.active.Divergence()
}

// NoiseVariance returns the noise power for SNR derivation.
func (bivariate *Bivariate) NoiseVariance() types.Scalar {
	if bivariate.active == nil {
		return 0
	}

	return bivariate.active.NoiseVariance()
}

// Support on State reports effective model support.
func (state *State) Support() float64 {
	if !state.modelReady {
		return 0
	}

	return state.modelSupport
}

// Divergence on State returns the signal departure.
func (state *State) Divergence() types.Scalar {
	if !state.hasSNR {
		return 0
	}

	return types.Scalar(math.Sqrt(state.snr))
}

// NoiseVariance on State returns the noise power.
func (state *State) NoiseVariance() types.Scalar {
	if !state.hasSNR {
		return 0
	}

	return 1.0
}

var (
	_ types.Node     = (*Bivariate)(nil)
	_ types.Reporter = (*Bivariate)(nil)
	_ types.Evidence = (*Bivariate)(nil)
	_ types.Rejector = (*Bivariate)(nil)
)
