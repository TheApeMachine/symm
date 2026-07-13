package logic

import (
	"math"
	"time"

	"github.com/theapemachine/datura"
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

var resonanceObservableKeys = []string{
	"pressure_grad_x",
	"divergence",
	"coherence_mag2",
	"guidance_speed",
	"stress_anisotropy",
}

type Resonance struct {
	symbol      string
	ui          chan []byte
	manifold    *learning.ResonanceManifold
	baselines   map[string]*adaptive.TimeElastic
	lastEventAt time.Time
	samples     uint64
}

func NewResonance(symbol string, ui chan []byte) *Resonance {
	arch := []int{resonanceObservables, resonanceObservables, resonanceObservables}
	manifold, err := learning.NewResonanceManifold(arch, 0, resonanceAlpha)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"logic resonance: failed to initialize manifold",
			err,
		))
	}

	resonance := &Resonance{
		symbol:    symbol,
		ui:        ui,
		manifold:  manifold,
		baselines: map[string]*adaptive.TimeElastic{},
	}

	return resonance
}

func (resonance *Resonance) Close() {}

func (resonance *Resonance) Update(thesis *types.Thesis) {
	if resonance.manifold == nil {
		return
	}

	stored, ok := thesis.Measurements.Load(resonance.symbol + ":manifold")

	if !ok {
		return
	}

	state := stored.(manifold.State)

	if !state.IsFinite() {
		return
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
		return
	}

	if resonance.eventStale(stepAt) {
		return
	}

	observables, ready := resonance.normalize(raw, stepAt)

	if !ready {
		return
	}

	resonance.advanceEventAt(stepAt)

	if err := resonance.manifold.Settle(observables, false); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic resonance: failed to settle manifold",
			err,
		))

		return
	}

	resonance.manifold.Learn(nil)
	resonance.samples++

	latent := resonance.manifold.LatentState()
	layers, surprise, energy := resonance.manifold.WireSnapshot()

	outcome := ResonanceOutcome{
		Source:      "resonance",
		Symbol:      resonance.symbol,
		At:          stepAt,
		Samples:     resonance.samples,
		Observables: observables,
		Latent:      latent,
		Layers:      layers,
		Energy:      energy,
		Surprise:    surprise,
	}

	if !outcome.IsFinite() {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic resonance: non-finite outcome",
			nil,
		))

		return
	}

	thesis.Measurements.Store(resonance.symbol+":resonance", outcome)

	if resonance.ui != nil {
		select {
		case resonance.ui <- datura.Map[any]{"resonance": outcome}.Marshal():
		default:
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic resonance: UI channel full while publishing outcome",
				nil,
			))
		}
	}
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
ResonanceOutcome is the online predictive-coding measurement for one manifold
state. It intentionally carries no return forecast: the current model has no
chronologically calibrated task head and therefore cannot be a strategy input.
*/
type ResonanceOutcome struct {
	Source      string                        `json:"source"`
	Symbol      string                        `json:"symbol"`
	At          time.Time                     `json:"at"`
	Samples     uint64                        `json:"samples"`
	Observables []float64                     `json:"observables"`
	Latent      []float64                     `json:"latent"`
	Layers      []learning.ResonanceLayerWire `json:"layers"`
	Energy      float64                       `json:"energy"`
	Surprise    float64                       `json:"surprise"`
}

func (outcome ResonanceOutcome) IsFinite() bool {
	if len(outcome.Observables) != resonanceObservables ||
		len(outcome.Latent) != resonanceObservables {
		return false
	}

	return finiteSlice(outcome.Observables) &&
		finiteSlice(outcome.Latent) &&
		finite(outcome.Energy) &&
		finite(outcome.Surprise)
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
