package logic

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
resonanceAlpha scales the adaptive learning-rate config the manifold derives
from its architecture.
*/
const resonanceAlpha = 0.01

/*
resonanceObservables is the dimensionality of the physical state consumed by
the online predictive-coding hierarchy.
*/
const resonanceObservables = 5

/*
returnFeatures is the return-head row. Arrival imbalance is the symbol-local
treatment for next-mid log return; shared-field scalars stay in predictive
coding and causal controls, not in this head, so multi-symbol pumps do not
train one RLS on contradictory peer Readings.
*/
const returnFeatures = 1

const resonanceReturnTarget = "next_l3_epoch_mid_log_return"

var resonanceObservableKeys = []string{
	"pressure_grad_x",
	"divergence",
	"coherence_mag2",
	"guidance_speed",
	"stress_anisotropy",
}

/*
Resonance learns the manifold's unsupervised physical representation and owns
a configured strict-prior RLS head for the next L3 midpoint return.
*/
type Resonance struct {
	symbol      string
	manifold    *learning.ResonanceManifold
	returns     *returnLadder
	baselines   map[string]*adaptive.TimeElastic
	halflife    time.Duration
	startedAt   time.Time
	lastEventAt time.Time
	lastMid     *decimal.Decimal
	samples     uint64
}

/*
NewResonance creates the symbol-local predictive-coding model whose outputs
Analyzer accumulates on the current Thesis.
*/
func NewResonance(
	symbol string,
	halflife time.Duration,
) *Resonance {
	if halflife <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic resonance: baseline halflife must be positive",
			nil,
		))

		return nil
	}

	arch := []int{resonanceObservables, resonanceObservables, resonanceObservables}
	manifoldOut, err := learning.NewResonanceManifold(arch, 1, resonanceAlpha)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"logic resonance: failed to initialize manifold",
			err,
		))

		return nil
	}

	returns, err := newReturnLadder()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic resonance: invalid return-head configuration",
			err,
		))

		return nil
	}

	resonance := &Resonance{
		symbol:    symbol,
		manifold:  manifoldOut,
		returns:   returns,
		baselines: map[string]*adaptive.TimeElastic{},
		halflife:  halflife,
	}

	return resonance
}

func (resonance *Resonance) Close() {}

/*
Update advances predictive coding from one coherent manifold state and returns
both durable measurements and its outcome for the current Thesis.
*/
func (resonance *Resonance) Update(
	state manifold.State,
) ([]*types.Measurement, *ResonanceOutcome) {
	if resonance.manifold == nil {
		return nil, nil
	}

	if !state.IsFinite() {
		return nil, nil
	}

	reading := state.Reading

	raw := []float64{
		reading.PressureGradX,
		reading.Divergence,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		state.StressAnisotropy,
	}

	stepAt := state.At

	if stepAt.IsZero() {
		return nil, nil
	}

	if resonance.eventStale(stepAt) {
		return nil, nil
	}

	observables, ready := resonance.normalize(raw, stepAt)

	if !ready {
		return nil, nil
	}

	resonance.advanceEventAt(stepAt)

	if err := resonance.manifold.Settle(observables, false); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to settle manifold",
			err,
		))

		return nil, nil
	}

	if err := resonance.manifold.Learn(nil); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: manifold learn failed",
			err,
		))

		return nil, nil
	}

	resonance.samples++

	if state.ReferencePrice == nil || state.ReferencePrice.Sign() <= 0 {
		return nil, nil
	}

	features, ok := resonance.forecastRow(state)

	if !ok {
		return nil, nil
	}

	// Identical mids carry no realized return; advancing would score zero
	// targets and inflate residual uncertainty without new information.
	if resonance.lastMid == nil || resonance.lastMid.Cmp(state.ReferencePrice) != 0 {
		if err := resonance.returns.Advance(
			features, state.ReferencePrice,
		); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"logic resonance: return ladder advance failed",
				err,
			))

			return nil, nil
		}

		resonance.lastMid = state.ReferencePrice.Copy()
	}

	forecast := resonance.returns.Forecast()
	latent := resonance.manifold.LatentState()
	layers, surprise, energy := resonance.manifold.WireSnapshot()

	outcome := ResonanceOutcome{
		Source:                     "resonance",
		Symbol:                     resonance.symbol,
		At:                         stepAt,
		Samples:                    resonance.samples,
		Observables:                observables,
		Latent:                     latent,
		Layers:                     layers,
		Energy:                     energy,
		Surprise:                   surprise,
		Target:                     resonanceReturnTarget,
		ExpectedReturn:             forecast.ExpectedReturn,
		ReturnReady:                forecast.Ready,
		HorizonEvents:              forecast.HorizonEvents,
		IncrementalMSE:             forecast.MeanMSE,
		IncrementalSkillLowerBound: forecast.SkillLower,
		Uncertainty:                forecast.Uncertainty,
		CalibrationSamples:         forecast.Samples,
	}

	if !outcome.IsFinite() {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic resonance: non-finite outcome",
			nil,
		))

		return nil, nil
	}

	horizon := state.Duration

	if horizon < 0 {
		horizon = 0
	}

	observedFrom := stepAt.Add(-horizon)

	elapsed := stepAt.Sub(resonance.startedAt)
	maturity := -math.Expm1(
		-math.Ln2 * float64(elapsed) /
			float64(resonance.halflife),
	)
	scale := types.ScaleReference{
		Kind: types.ScaleObservationWindow, From: observedFrom, Through: stepAt,
	}
	validity := types.MeasurementValidity{
		State: types.ValidityValid, Readiness: types.ReadinessModel,
	}

	return []*types.Measurement{
		{
			Source:       types.SourceResonance,
			Symbol:       resonance.symbol,
			At:           stepAt,
			ObservedFrom: observedFrom,
			Horizon:      horizon,
			Maturity:     maturity,
			Validity:     validity,
			Scale:        scale,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricResonanceEnergy, types.SideNone): {
					Raw:  outcome.Energy,
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricResonanceSurprise, types.SideNone): {
					Raw:  outcome.Surprise,
					Unit: types.UnitNat,
				},
			},
		},
	}, &outcome
}

func (resonance *Resonance) normalize(
	observables []float64,
	at time.Time,
) ([]float64, bool) {
	if len(observables) != resonanceObservables || at.IsZero() {
		return nil, false
	}

	normalized := make([]float64, len(observables))
	allReady := true

	for index, value := range observables {
		if !finite(value) {
			return nil, false
		}

		if value == 0 {
			continue
		}

		key := "resonance/" + resonanceObservableKeys[index]
		tracker := resonance.baselines[key]

		if tracker == nil {
			tracker = adaptive.NewTimeElastic(adaptive.TimeElasticConfig{
				Halflife: resonance.halflife,
				Epsilon:  manifold.BaselineEpsilon,
			})
			resonance.baselines[key] = tracker
		}

		output, err := tracker.Measure(adaptive.TimedValue{
			Value: math.Abs(value),
			At:    at,
		})

		if err != nil {
			errnie.Error(err)

			return nil, false
		}

		if !output.Ready {
			allReady = false

			continue
		}

		normalized[index] = math.Log(output.Value) * math.Copysign(1, value)
	}

	return normalized, allReady
}

/*
forecastRow builds the return-head treatment: symbol-local buy/sell arrival
imbalance for the next-mid log return.
*/
func (resonance *Resonance) forecastRow(state manifold.State) ([]float64, bool) {
	arrivalMass := state.BuyIntensity + state.SellIntensity

	if arrivalMass <= 0 {
		return nil, false
	}

	imbalance := (state.BuyIntensity - state.SellIntensity) / arrivalMass

	if !finite(imbalance) {
		return nil, false
	}

	return []float64{imbalance}, true
}

func (resonance *Resonance) eventStale(at time.Time) bool {
	if at.IsZero() || resonance.lastEventAt.IsZero() {
		return false
	}

	return at.Before(resonance.lastEventAt)
}

func (resonance *Resonance) advanceEventAt(at time.Time) {
	if at.IsZero() {
		return
	}

	if resonance.startedAt.IsZero() {
		resonance.startedAt = at
	}

	if resonance.lastEventAt.IsZero() || at.After(resonance.lastEventAt) {
		resonance.lastEventAt = at
	}
}

/*
ResonanceOutcome is the online predictive-coding measurement for one manifold
state, including the chronologically trained next-midpoint return task head.
*/
type ResonanceOutcome struct {
	Source                     string                        `json:"source"`
	Symbol                     string                        `json:"symbol"`
	At                         time.Time                     `json:"at"`
	Samples                    uint64                        `json:"samples"`
	Observables                []float64                     `json:"observables"`
	Latent                     []float64                     `json:"latent"`
	Layers                     []learning.ResonanceLayerWire `json:"layers"`
	Energy                     float64                       `json:"energy"`
	Surprise                   float64                       `json:"surprise"`
	Target                     string                        `json:"target"`
	ExpectedReturn             float64                       `json:"expectedReturn"`
	ReturnReady                bool                          `json:"returnReady"`
	HorizonEvents              uint64                        `json:"horizonEvents"`
	IncrementalMSE             float64                       `json:"incrementalMSE"`
	IncrementalSkillLowerBound float64                       `json:"incrementalSkillLowerBound"`
	Uncertainty                float64                       `json:"uncertainty"`
	CalibrationSamples         uint64                        `json:"calibrationSamples"`
}

func (outcome ResonanceOutcome) IsFinite() bool {
	if len(outcome.Observables) != resonanceObservables ||
		len(outcome.Latent) != resonanceObservables {
		return false
	}

	return finiteSlice(outcome.Observables) &&
		finiteSlice(outcome.Latent) &&
		finite(outcome.Energy) &&
		finite(outcome.Surprise) &&
		finite(outcome.ExpectedReturn) &&
		finite(outcome.IncrementalMSE) &&
		finite(outcome.IncrementalSkillLowerBound) &&
		finite(outcome.Uncertainty)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteSlice(values []float64) bool {
	for _, value := range values {
		if !finite(value) {
			return false
		}
	}

	return true
}
