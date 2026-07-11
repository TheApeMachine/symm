package logic

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/strategy"
)

/*
resonanceAlpha scales the adaptive learning-rate config the manifold derives
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
	symbol      string
	thesis      *strategy.Thesis
	ui          chan []byte
	manifold    *learning.ResonanceManifold
	horizon     time.Duration
	pending     []pendingForecast
	baselines   map[string]*adaptive.TimeElastic
	lastEventAt time.Time
	lastPriceAt time.Time
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

func NewResonance(symbol string, thesis *strategy.Thesis, ui chan []byte) *Resonance {
	arch := []int{resonanceObservables, resonanceObservables, resonanceObservables}
	manifold, err := learning.NewResonanceManifold(arch, resonancePriceTarget, resonanceAlpha)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"logic resonance: failed to initialize manifold",
			err,
		))
	}

	resonance := &Resonance{
		symbol:    symbol,
		thesis:    thesis,
		ui:        ui,
		horizon:   viper.GetViper().GetDuration(resonanceForwardReturnHorizonKey),
		manifold:  manifold,
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

func (resonance *Resonance) Close() {}

func (resonance *Resonance) Update() *strategy.Thesis {
	if resonance.manifold == nil {
		return resonance.thesis
	}

	snapshot, ok := resonance.thesis.Evidence(resonance.symbol, "manifold")

	if !ok {
		return resonance.thesis
	}

	state, ok := manifold.StateFromEvidence(snapshot)

	if !ok || !state.IsFinite() {
		return resonance.thesis
	}

	reading := state.Reading

	raw := []float64{
		reading.PressureGradNorm,
		reading.Divergence,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
	}

	stepAt, hasStep := resonance.StepAt()

	if !hasStep {
		return resonance.thesis
	}

	if resonance.eventStale(stepAt) {
		return resonance.thesis
	}

	observables, ready := resonance.normalize(raw, stepAt)

	if !ready {
		return resonance.thesis
	}

	resonance.advanceEventAt(stepAt)

	price, priceAt, hasPrice := resonance.Price()

	if hasPrice && price > 0 && resonance.horizon > 0 {
		if resonance.lastPriceAt.IsZero() || priceAt.After(resonance.lastPriceAt) {
			resonance.pending = append(resonance.pending, pendingForecast{
				input: observables,
				price: price,
				at:    priceAt,
			})

			resonance.lastPriceAt = priceAt

			for len(resonance.pending) > 0 &&
				!priceAt.Before(resonance.pending[0].at.Add(resonance.horizon)) {
				matured := resonance.pending[0]
				resonance.pending = resonance.pending[1:]

				if matured.price > 0 && priceAt.After(matured.at) {
					resonance.learnForwardReturn(matured.input, math.Log(price/matured.price))
				}
			}
		}
	}

	if err := resonance.manifold.Settle(observables, true); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to settle manifold",
			err,
		))

		return resonance.thesis
	}

	latent := resonance.manifold.LatentState()
	energy := resonance.manifold.Energy()
	surprise := resonance.manifold.ReconstructionError()

	forecast := 0.0
	prediction := resonance.manifold.TaskPrediction()

	if len(prediction) > 0 {
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

	resonance.thesis.AddEvidence(resonance.symbol, "resonance", outcome)

	if resonance.ui != nil {
		frame := datura.Map[any]{"resonance": outcome}

		if symbol, ok := resonance.thesis.Evidence(resonance.symbol, "symbol"); ok {
			frame["symbol"] = symbol
		}

		select {
		case resonance.ui <- frame.Marshal():
		default:
		}
	}

	return resonance.thesis
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
				Halflife: manifold.DefaultBaselineHalflife,
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
	if err := resonance.manifold.Settle(input, true); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to settle supervised sample",
			err,
		))

		return
	}

	resonance.manifold.Learn([]float64{forwardReturn})
}

/*
StepAt reads the current step timestamp the manifold recorded on the thesis.
*/
func (resonance *Resonance) StepAt() (time.Time, bool) {
	snapshot, ok := resonance.thesis.Evidence(resonance.symbol, "step_at")

	if !ok {
		return time.Time{}, false
	}

	at, ok := snapshot.(time.Time)

	if !ok || at.IsZero() {
		return time.Time{}, false
	}

	return at, true
}

/*
Price reads the current mid price the manifold recorded on the thesis.
*/
func (resonance *Resonance) Price() (float64, time.Time, bool) {
	snapshot, ok := resonance.thesis.Evidence(resonance.symbol, "price")

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

	atSnapshot, ok := resonance.thesis.Evidence(resonance.symbol, "price_at")

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
