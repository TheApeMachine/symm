package store

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"github.com/theapemachine/errnie"
	"math"
)

/*
	ResonanceServer explicitly retains the learned manifold and causal forecast

ledger for one graph-factory market scope. Inputs and outputs stay native Cap'n
Proto; no source observation or future outcome enters an earlier forecast.
*/
type ResonanceServer struct {
	manifold     *resonanceManifold
	identities   []string
	memory       resonanceMemory
	pending      []*resonanceReference
	observations int
	resolved     int
	epoch        int64
	sequence     int64
	reference    float64
	returnCount  int
	returnMean   float64
	returnM2     float64
	alpha        float64
	out          resonanceOutput
}

type resonanceReference struct {
	reference   float64
	features    []float64
	predictions []resonanceForecast
	issued      int
	noise       float64
}

type resonanceResolution struct {
	Prediction, Target, Error float64
	Horizon                   int
	Step                      int64
}
type resonanceDynamics struct {
	Energy, PredictionEnergy, ReconstructionError, TemporalError, Alpha float64
	HasTemporalError                                                    bool
}
type resonanceOutput struct {
	Dynamics                       *resonanceDynamics
	Reading                        *resonanceReading
	Forecast                       []resonanceForecast
	ForwardCurve, ForwardRetention []float64
	SupportedHorizon               int
	Calibrated                     bool
	ResolvedSteps, Pending         int
	Readout                        []float64
	Confidence                     float64
	LastResolution                 *resonanceResolution
}

/* NewResonance constructs an idle retained owner; observed input establishes its dimensions. */
func NewResonance() *ResonanceServer { return &ResonanceServer{} }

/*
	Write settles the current complete feature vector, resolves the previously

issued next-observation forecast, then issues the next one from current state.
*/
func (server *ResonanceServer) Write(ctx context.Context, call Resonance_write) error {
	args := call.Args()
	features, err := args.Features()
	if err != nil {
		return errnie.Error(err)
	}
	if features.Len() == 0 || args.Reference() <= 0 || args.Epoch() < 0 || args.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "resonance: features, positive reference and causal stamps required", nil))
	}
	identities, err := args.FeatureIdentities()

	if err != nil {
		return errnie.Error(err)
	}

	if err := server.identify(identities, features.Len()); err != nil {
		return err
	}
	if server.manifold != nil && args.Epoch() == server.epoch && args.Sequence() <= server.sequence {
		return errnie.Error(errnie.Err(errnie.Validation, "resonance: observation sequence did not advance", nil))
	}
	if server.manifold != nil && args.Epoch() < server.epoch {
		return errnie.Error(errnie.Err(errnie.Validation, "resonance: epoch regressed", nil))
	}
	if server.manifold != nil && args.Epoch() != server.epoch {
		// A new source epoch severs temporal adjacency, not learned evidence.
		// Outstanding predictions cannot be labelled across an unobserved gap.
		server.pending = nil
		server.reference = 0
		server.memory.previous = nil
		server.manifold.temporalPriorsReady = false
		server.out = resonanceOutput{}
	}
	hasPrevious := server.reference > 0
	input := make([]float64, features.Len())
	for index := range input {
		input[index] = features.At(index)
	}
	server.epoch = args.Epoch()
	server.sequence = args.Sequence()
	server.observations++
	// Reciprocal support is the exact running-average gain, not a chosen market window.
	server.alpha = 1 / float64(server.observations)
	if server.manifold == nil {
		width := len(input)
		server.manifold = newResonanceManifold([]int{width, width, width}, 1, server.alpha, readoutLatents)
	}
	if _, err := server.manifold.Execute(resonanceCommand{Alpha: &resonanceAlpha{Alpha: server.alpha}}); err != nil {
		return errnie.Error(err)
	}
	if _, err := server.manifold.Execute(resonanceCommand{Batch: &resonanceBatch{Input: input, Learn: true}}); err != nil {
		return errnie.Error(err)
	}
	if err := server.resolve(args.Reference()); err != nil {
		return err
	}
	server.manifold.extend(server.memory.observe(input))
	reading, err := server.manifold.Execute(resonanceCommand{Reading: &resonanceInspect{}})
	if err != nil {
		return errnie.Error(err)
	}
	reading.HasTemporalError = hasPrevious
	forecast, err := server.manifold.Execute(resonanceCommand{Forecast: &resonanceQuery{Steps: server.manifold.taskRows}})
	if err != nil {
		return errnie.Error(err)
	}
	output := resonanceOutput{Reading: &reading, Forecast: forecast.Forecast, Readout: reading.Readout, ResolvedSteps: server.resolved, LastResolution: server.out.LastResolution,
		Dynamics: &resonanceDynamics{Energy: reading.Energy, PredictionEnergy: reading.PredictionEnergy, ReconstructionError: reading.ReconstructionError, TemporalError: reading.TemporalError, HasTemporalError: reading.HasTemporalError, Alpha: server.alpha}}
	for _, prediction := range forecast.Forecast {
		if !prediction.Ready {
			break
		}
		output.SupportedHorizon++
		output.ForwardCurve = append(output.ForwardCurve, prediction.Value)
	}
	output.Calibrated = output.SupportedHorizon > 0
	output.ForwardRetention = server.manifold.retentionVector()
	if len(reading.Skill) > 0 && reading.SkillReady[0] {
		output.Confidence = reading.Skill[0]
	}
	if server.returnCount > 1 && server.returnM2 > 0 {
		server.pending = append(server.pending, &resonanceReference{reference: args.Reference(), features: append([]float64(nil), reading.Readout...), predictions: forecast.Forecast, issued: server.observations, noise: math.Sqrt(server.returnM2 / float64(server.returnCount-1))})
	}
	output.Pending = len(server.pending)
	server.reference = args.Reference()
	server.out = output
	return nil
}

/* resolve uses the noise scale stored when the forecast was issued. */
func (server *ResonanceServer) resolve(reference float64) error {
	if server.reference > 0 {
		observed := math.Log(reference / server.reference)
		server.returnCount++
		delta := observed - server.returnMean
		server.returnMean += delta / float64(server.returnCount)
		server.returnM2 += delta * (observed - server.returnMean)
	}
	retained := server.pending[:0]
	for _, prior := range server.pending {
		horizon := server.observations - prior.issued
		if horizon > len(prior.predictions) {
			continue
		}
		change := math.Log(reference / prior.reference)
		target := 0.0
		// Diffusive noise over h increments grows as sqrt(h); the per-step scale
		// was measured and captured at issue time.
		noise := prior.noise * math.Sqrt(float64(horizon))
		if change > noise {
			target = 1
		}
		if change < -noise {
			target = -1
		}
		prediction := prior.predictions[horizon-1]
		if err := server.manifold.observeTask(horizon, prior.features, prediction.Value, target, prediction.Ready); err != nil {
			return errnie.Error(err)
		}
		server.resolved++
		server.out.LastResolution = &resonanceResolution{Prediction: prediction.Value, Target: target, Error: target - prediction.Value, Horizon: horizon, Step: server.sequence}
		if horizon < len(prior.predictions) {
			retained = append(retained, prior)
		}
	}
	server.pending = retained
	return nil
}

func floatsOf(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, errnie.Error(err)
	}
	values := make([]float64, list.Len())
	for index := range values {
		values[index] = list.At(index)
	}
	return values, nil
}
