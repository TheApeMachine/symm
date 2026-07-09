package logic

import (
	"math"

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
resonanceForecastHorizon is how many ticks ahead the supervised third (task) head
predicts the forward return: this tick's inputs are trained against the log return
realized resonanceForecastHorizon ticks later.
*/
const resonanceForecastHorizon = 8

/*
resonancePriceTarget is the task-head dimensionality: a single scalar, the
forward log return.
*/
const resonancePriceTarget = 1

type Resonance struct {
	thesis  *strategy.Thesis
	solver  *rmanifold.BatchSolver
	pending []pendingForecast
}

/*
pendingForecast holds one tick's solver input and the price observed at that tick,
so the task head can be supervised against the log return once the forward price
arrives resonanceForecastHorizon ticks later.
*/
type pendingForecast struct {
	input []float64
	price float64
}

func NewResonance(thesis *strategy.Thesis) *Resonance {
	// The resonance layer models the manifold's live post-step observables (one
	// batch slot). The third head is the supervised task head predicting the
	// forward log return, so targetDim is that single scalar.
	arch := []int{resonanceObservables, resonanceObservables, resonanceObservables}

	resonance := &Resonance{
		thesis: thesis,
		solver: rmanifold.NewBatchSolver(
			arch, resonancePriceTarget, 1, resonanceAlpha,
		),
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

	price, hasPrice := resonancePrice(resonance.thesis)

	// Supervise the task head: the price observed now realizes the forward return
	// for the input buffered resonanceForecastHorizon ticks ago. The target is the
	// log return ln(P_now / P_then), which is stationary and lies within the
	// tanh output range the task head squashes through — raw price would saturate.
	if hasPrice && price > 0 {
		resonance.pending = append(resonance.pending, pendingForecast{
			input: observables,
			price: price,
		})

		if len(resonance.pending) > resonanceForecastHorizon {
			matured := resonance.pending[0]
			resonance.pending = resonance.pending[1:]

			if matured.price > 0 {
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
resonancePrice reads the current mid price the manifold recorded on the thesis.
*/
func resonancePrice(thesis *strategy.Thesis) (float64, bool) {
	snapshot, ok := thesis.Evidence("price")

	if !ok {
		return 0, false
	}

	price, ok := snapshot.(float64)

	return price, ok
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
