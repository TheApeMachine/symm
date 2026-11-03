package resonance

import (
	"context"
	"fmt"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Solver runs one predictive-coding manifold per symbol over that symbol's
signal features, and publishes the settled reading under Resonance[symbol].
*/
type Solver struct {
	*runtime.System
	symbols *sync.Map
	pace    float64
}

/*
Feature is one signal Map() metric the manifold observes.
*/
type Feature struct {
	Source string
	Label  string
}

/*
symbolState is one symbol's coder, feature moments, and deferred target.
*/
type symbolState struct {
	manifold *learning.ResonanceManifold
	scorer   *featureScorer
	pending  []float64
}

func NewSolver(ctx context.Context) *Solver {
	solver := &Solver{
		System:  runtime.NewSystem(ctx, "resonance"),
		symbols: &sync.Map{},
		pace:    system.Cfg.Resonance.LearningRate,
	}

	solver.Transition(runtime.READY)
	return solver
}

/*
Step settles every symbol that carries a full feature vector this tick.
*/
func (solver *Solver) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		errnie.Error(errnie.Err(errnie.Validation, "resonance: Step needs an envelope", nil))
		return nil
	}

	if err := solver.Error(); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "resonance: solver failed earlier", err))
		return envelope
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for symbol, sources := range envelope.Metrics() {
		if symbol == "" || symbol == "aggregate" {
			continue
		}

		symbol, sources := symbol, sources
		wg.Add(1)

		task := func() {
			defer wg.Done()

			reading := solver.stepSymbol(symbol, sources)
			if reading == nil {
				return
			}

			mu.Lock()
			envelope.Resonance[symbol] = reading
			mu.Unlock()

			ui.Focus("resonance", symbol, reading)
		}

		if system.Q.Submit(task) != nil {
			task()
		}
	}

	wg.Wait()
	return envelope
}

/*
stepSymbol standardizes a complete observation, settles the manifold, and
trains the supervised head on the prior epoch once this epoch has arrived.
*/
func (solver *Solver) stepSymbol(
	symbol string,
	sources map[string]map[string]float64,
) map[string]float64 {
	if err := solver.Error(); err != nil {
		return nil
	}

	state := solver.state(symbol)
	if state == nil {
		return nil
	}

	input, ready := state.scorer.Observe(sources)
	if !ready {
		return nil
	}

	target := state.pending
	hadTarget := len(target) == len(input)

	if !hadTarget {
		target = nil
	}

	if _, err := state.manifold.SettleFromBatchOptions(input, target, true, false); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("resonance: %s settle failed", symbol),
			err,
		))

		return nil
	}

	state.pending = append([]float64(nil), input...)
	return solver.publish(state.manifold, hadTarget)
}

func (solver *Solver) state(symbol string) *symbolState {
	if held, ok := solver.symbols.Load(symbol); ok {
		return held.(*symbolState)
	}

	manifold := learning.NewResonanceManifold(
		coderArch(len(Features)),
		len(Features),
		solver.pace,
	)

	if manifold == nil {
		solver.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: unable to build manifold for %s", symbol),
			nil,
		))

		return nil
	}

	state := &symbolState{
		manifold: manifold,
		scorer:   newFeatureScorer(),
	}

	loaded, _ := solver.symbols.LoadOrStore(symbol, state)
	return loaded.(*symbolState)
}

/*
featureScorer standardizes each feature against its own prior moments and
holds the last raw reading per Feature. A single trade tick almost never
carries every signal source at once; holding lets the manifold settle once
the full set has been seen across ticks.
*/
type featureScorer struct {
	standardizers []*adaptive.Standardizer
	held          []float64
	seen          []bool
}

func newFeatureScorer() *featureScorer {
	scorer := &featureScorer{
		standardizers: make([]*adaptive.Standardizer, len(Features)),
		held:          make([]float64, len(Features)),
		seen:          make([]bool, len(Features)),
	}

	for i := range scorer.standardizers {
		scorer.standardizers[i] = adaptive.NewStandardizer()
	}

	return scorer
}

/*
Observe folds whatever labels arrived this tick into the held vector. It
returns a standardized input only when every Feature has been seen at least
once, at least one label updated this tick, and at least one standardizer
is ready.
*/
func (scorer *featureScorer) Observe(
	sources map[string]map[string]float64,
) ([]float64, bool) {
	if sources == nil {
		return nil, false
	}

	updated := false

	for i, feature := range Features {
		labels := sources[feature.Source]
		if labels == nil {
			continue
		}

		value, ok := labels[feature.Label]
		if !ok {
			continue
		}

		scorer.held[i] = value
		scorer.seen[i] = true
		updated = true
	}

	if !updated {
		return nil, false
	}

	for _, ok := range scorer.seen {
		if !ok {
			return nil, false
		}
	}

	input := make([]float64, len(Features))
	ready := false

	for i, reading := range scorer.held {
		output, err := scorer.standardizers[i].Measure(reading)
		if err != nil {
			return nil, false
		}

		input[i] = output.Value

		if output.Ready {
			ready = true
		}
	}

	return input, ready
}

func coderArch(featureDim int) []int {
	layers := system.Cfg.Resonance.Layers
	arch := make([]int, layers)
	arch[0] = featureDim

	for i := 1; i < layers; i++ {
		arch[i] = featureDim * 2
	}

	arch[layers-1] = featureDim
	return arch
}

func (solver *Solver) publish(
	manifold *learning.ResonanceManifold,
	hadTarget bool,
) map[string]float64 {
	layers, surprise, energy := manifold.WireSnapshot()
	latent := manifold.LatentState()

	reading := map[string]float64{
		"surprise": surprise,
		"energy":   energy,
		"alpha":    solver.pace,
	}

	if skill, ok := manifold.TaskSkill(); ok {
		reading["task_skill"] = skill
		reading["calibrated"] = 1
	} else {
		reading["calibrated"] = 0
	}

	if precision, ok := manifold.TaskPrecision(); ok {
		reading["task_relative_precision"] = precision
	}

	if temporal, ok := manifold.TemporalError(); ok {
		reading["temporal_error"] = temporal
	}

	for i, val := range latent {
		reading[fmt.Sprintf("latent_%d", i)] = val
	}

	for layer, wire := range layers {
		reading[fmt.Sprintf("layer_%d_error", layer)] = wire.ErrorNorm

		if wire.Temporal {
			reading[fmt.Sprintf("layer_%d_temporal", layer)] = 1
		} else {
			reading[fmt.Sprintf("layer_%d_temporal", layer)] = 0
		}

		for j, val := range wire.State {
			reading[fmt.Sprintf("layer_%d_state_%d", layer, j)] = val
		}

		for j, val := range wire.Prediction {
			reading[fmt.Sprintf("layer_%d_prediction_%d", layer, j)] = val
		}
	}

	curve := manifold.TaskPrediction()

	for i, val := range curve {
		reading[fmt.Sprintf("forward_curve_%d", i)] = val
	}

	supported := 0.0

	if hadTarget && len(curve) > 0 {
		supported = float64(len(curve))
	}

	reading["supported_horizon"] = supported
	return reading
}

func (solver *Solver) Reset(resetPrecision bool) {
	solver.symbols.Range(func(_, held any) bool {
		state := held.(*symbolState)
		state.manifold.ResetState(resetPrecision)
		state.scorer = newFeatureScorer()
		state.pending = nil
		return true
	})
}

func (solver *Solver) Close() error {
	solver.symbols.Clear()
	return nil
}
