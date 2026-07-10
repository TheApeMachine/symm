package logic

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	rmanifold "github.com/theapemachine/nomagique/learning/manifold"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/strategy"
)

/*
resonanceAlpha scales the adaptive learning-rate config the batch solver derives
from its architecture.
*/
const resonanceAlpha = 0.01

/*
resonanceObservables is the dimensionality of the manifold Reading the resonance
layer consumes. pmanifold's step: reading hardcodes pressure_grad_{x,y,z} to zero
(only the pooled pressure_grad_norm/divergence carry the spatial signal), so those
three dead channels are excluded — the live observables are pressure_grad_norm,
divergence, coherence_mag2, guidance_speed, and viscosity_proxy.
*/
const resonanceObservables = 5

/*
resonanceForwardReturnHorizonKey is the configured wall-clock horizon the
supervised task head predicts.
*/
const resonanceForwardReturnHorizonKey = "trading.edge.forward_return_horizon"

/*
resonancePriceTarget is the task-head dimensionality: a single scalar, the
forward log return.
*/
const resonancePriceTarget = 1

var resonanceObservableKeys = []string{
	"pressure_grad_norm",
	"divergence",
	"coherence_mag2",
	"guidance_speed",
	"viscosity_proxy",
}

type Resonance struct {
	thesis      *strategy.Thesis
	solver      *rmanifold.BatchSolver
	horizon     time.Duration
	pending     []pendingForecast
	baselines   map[string]*adaptive.TimeElastic
	lastEventAt time.Time
}

/*
pendingForecast holds one solver input and the price observed at that event time,
so the task head can be supervised against the log return once the configured
forward horizon has elapsed.
*/
type pendingForecast struct {
	input []float64
	price float64
	at    time.Time
}

func NewResonance(thesis *strategy.Thesis) *Resonance {
	// The resonance layer models the manifold's live post-step observables (one
	// batch slot). The third head is the supervised task head predicting the
	// forward log return, so targetDim is that single scalar.
	solver, err := newResonanceSolver()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"logic resonance: failed to initialize solver",
			err,
		))
	}

	resonance := &Resonance{
		thesis:    thesis,
		horizon:   viper.GetViper().GetDuration(resonanceForwardReturnHorizonKey),
		solver:    solver,
		baselines: map[string]*adaptive.TimeElastic{},
	}

	if resonance.horizon <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic resonance: trading.edge.forward_return_horizon must be positive",
			nil,
		))
	}

	return resonance
}

/*
Close releases the GPU-backed batch solver the resonance layer owns.
*/
func (resonance *Resonance) Close() {
	if resonance.solver != nil {
		resonance.solver.Close()
	}
}

func (resonance *Resonance) Update() *strategy.Thesis {
	if resonance.solver == nil {
		return resonance.thesis
	}

	snapshot, ok := resonance.thesis.Evidence("manifold")

	if !ok {
		return resonance.thesis
	}

	reading, ok := snapshot.(pmanifold.Reading)

	if !ok || !reading.IsFinite() {
		return resonance.thesis
	}

	raw := []float64{
		reading.PressureGradNorm,
		reading.Divergence,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
	}

	price, priceAt, hasPrice := resonance.Price()

	if resonance.eventStale(priceAt) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic resonance: event timestamp must not regress",
			nil,
		))

		return resonance.thesis
	}

	observables, ready := resonance.normalize(raw, priceAt)

	if !ready {
		return resonance.thesis
	}

	resonance.advanceEventAt(priceAt)

	// Supervise the task head: the price observed now realizes the forward return
	// for each input whose configured wall-clock horizon has elapsed. The target
	// is the log return ln(P_now / P_then), which is stationary and lies within the
	// tanh output range the task head squashes through — raw price would saturate.
	if hasPrice && price > 0 && resonance.horizon > 0 {
		resonance.pending = append(resonance.pending, pendingForecast{
			input: observables,
			price: price,
			at:    priceAt,
		})

		for len(resonance.pending) > 0 &&
			!priceAt.Before(resonance.pending[0].at.Add(resonance.horizon)) {
			matured := resonance.pending[0]
			resonance.pending = resonance.pending[1:]

			if matured.price > 0 && priceAt.After(matured.at) {
				resonance.learnForwardReturn(matured.input, math.Log(price/matured.price))
			}
		}
	}

	// Forecast: settle the current observables and read the task head's forward
	// price prediction.
	if err := resonance.solver.SetInputs(observables, nil); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to set solver inputs",
			err,
		))

		return resonance.thesis
	}

	if err := resonance.solver.Settle(true); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to settle solver",
			err,
		))

		return resonance.thesis
	}

	if err := resonance.solver.ReadOutcomes(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to read outcomes",
			err,
		))

		return resonance.thesis
	}

	latent, energy, surprise, err := resonance.solver.OutcomeSlot(0)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to read outcome slot",
			err,
		))

		return resonance.thesis
	}

	forecast := 0.0

	prediction, err := resonance.solver.TaskPrediction(0, latent)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to read task prediction",
			err,
		))
	} else if len(prediction) > 0 {
		forecast = prediction[0]
	}

	outcome := ResonanceOutcome{
		Latent:         latent,
		Energy:         energy,
		Surprise:       surprise,
		ReturnForecast: forecast,
	}

	if !outcome.IsFinite() {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic resonance: non-finite outcome",
			nil,
		))

		return resonance.thesis
	}

	resonance.thesis.AddEvidence("resonance", outcome)

	return resonance.thesis
}

func newResonanceSolver() (*rmanifold.BatchSolver, error) {
	arch := []int{resonanceObservables, resonanceObservables, resonanceObservables}
	var solver *rmanifold.BatchSolver

	err := compute.WithMetalInit(func() error {
		solver = rmanifold.NewBatchSolver(
			arch, resonancePriceTarget, 1, resonanceAlpha,
		)

		if solver == nil {
			return errnie.Err(
				errnie.Internal,
				"logic resonance: solver was not created",
				nil,
			)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return solver, nil
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
				Halflife: defaultBaselineHalflife,
				Epsilon:  baselineEpsilon,
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

		normalized[index] = math.Copysign(output.Value-1, value)
	}

	return normalized, allReady
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

	if resonance.lastEventAt.IsZero() || at.After(resonance.lastEventAt) {
		resonance.lastEventAt = at
	}
}

/*
learnForwardReturn supervises the task head: settle the matured input, then learn
against its realized forward log return.
*/
func (resonance *Resonance) learnForwardReturn(input []float64, forwardReturn float64) {
	if err := resonance.solver.SetInputs(input, []float64{forwardReturn}); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to set supervised inputs",
			err,
		))

		return
	}

	if err := resonance.solver.Settle(false); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to settle supervised sample",
			err,
		))

		return
	}

	if err := resonance.solver.Learn(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to learn forward return",
			err,
		))
	}
}

/*
Price reads the current mid price the manifold recorded on the thesis.
*/
func (resonance *Resonance) Price() (float64, time.Time, bool) {
	snapshot, ok := resonance.thesis.Evidence("price")

	if !ok {
		return 0, time.Time{}, false
	}

	price, ok := snapshot.(float64)

	if !ok {
		return 0, time.Time{}, false
	}

	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, time.Time{}, false
	}

	atSnapshot, ok := resonance.thesis.Evidence("price_at")

	if !ok {
		return 0, time.Time{}, false
	}

	at, ok := atSnapshot.(time.Time)

	if !ok || at.IsZero() {
		return 0, time.Time{}, false
	}

	return price, at, true
}

/*
ResonanceOutcome is the resonance solver's per-step result: the settled latent
state, its free-energy and surprise scalars, and the task head's forward log
return forecast.
*/
type ResonanceOutcome struct {
	Latent         []float64
	Energy         float64
	Surprise       float64
	ReturnForecast float64
}

func (outcome ResonanceOutcome) IsFinite() bool {
	if len(outcome.Latent) != resonanceObservables {
		return false
	}

	return finiteSlice(outcome.Latent) &&
		finite(outcome.Energy) &&
		finite(outcome.Surprise) &&
		finite(outcome.ReturnForecast)
}
