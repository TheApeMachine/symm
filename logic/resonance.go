package logic

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	rmanifold "github.com/theapemachine/nomagique/learning/manifold"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
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

type Resonance struct {
	thesis  *strategy.Thesis
	solver  *rmanifold.BatchSolver
	horizon time.Duration
	pending []pendingForecast
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
	arch := []int{resonanceObservables, resonanceObservables, resonanceObservables}

	resonance := &Resonance{
		thesis:  thesis,
		horizon: viper.GetViper().GetDuration(resonanceForwardReturnHorizonKey),
		solver: rmanifold.NewBatchSolver(
			arch, resonancePriceTarget, 1, resonanceAlpha,
		),
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
	resonance.solver.Close()
}

func (resonance *Resonance) Update() *strategy.Thesis {
	snapshot, ok := resonance.thesis.Evidence("manifold")

	if !ok {
		return resonance.thesis
	}

	reading, ok := snapshot.(pmanifold.Reading)

	if !ok || !reading.IsFinite() {
		return resonance.thesis
	}

	observables := []float64{
		reading.PressureGradNorm,
		reading.Divergence,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
	}

	price, priceAt, hasPrice := resonance.Price()

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

	resonance.thesis.AddEvidence("resonance", ResonanceOutcome{
		Latent:         latent,
		Energy:         energy,
		Surprise:       surprise,
		ReturnForecast: forecast,
	})

	return resonance.thesis
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
