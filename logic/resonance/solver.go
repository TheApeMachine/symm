package resonance

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Solver runs one predictive-coding manifold per symbol over that symbol's
signal features (Features), and publishes the settled reading on the envelope
under Resonance[symbol] for the strategy stage and the dashboard to read.

The manifold is the CPU predictive coder in nomagique/learning: it settles a
latent state that explains the current observation (its residual is Surprise),
carries a temporal prior across ticks, and carries a supervised task head
whose target is the next observation's feature vector.
*/
type Solver struct {
	*runtime.System
	manifolds *sync.Map
	pace      float64
	price     *broker.Price

	// standardizers hold each symbol's per-feature moments, so a feature is
	// scored against its own history rather than the raw feed.
	standardizers *sync.Map

	// pending holds, per symbol, the feature vector issued at the previous
	// analysis epoch. Its target is the current vector: the supervised head
	// trains on the future once it has actually arrived.
	pending *sync.Map
}

/*
Feature is one grid-pinned metric (see features.go for the ordered Features slice).
*/
type Feature struct {
	Source string
	Label  string
	// Scope "aggregate" reads source/aggregate/<label>; empty uses the symbol.
	Scope string
}

/*
NewSolver returns a solver stepping at pace. price supplies broker oracle
features (Volume / Day / Spread / Book); nil skips those dims until set.
*/
func NewSolver(ctx context.Context, price *broker.Price) *Solver {
	solver := &Solver{
		System:        runtime.NewSystem(ctx, "resonance"),
		manifolds:     &sync.Map{},
		pace:          system.Cfg.Resonance.LearningRate,
		price:         price,
		standardizers: &sync.Map{},
		pending:       &sync.Map{},
	}

	solver.Transition(runtime.READY)
	return solver
}

/*
Step advances every symbol on the envelope through its predictive coder and
attaches scalar readings under envelope.Resonance[symbol].
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

	metrics := envelope.Metrics()

	for _, symbol := range symbolsOnEnvelope(envelope, metrics) {
		reading := solver.stepSymbol(metrics, symbol)

		if reading == nil {
			continue
		}

		envelope.Resonance[symbol] = reading

		if types.Allows(symbol) && ui.UI != nil && ui.UI.Active() {
			ui.UI.WriteJSON(map[string]any{
				"resonance": map[string]any{symbol: reading},
			})
		}
	}

	return envelope
}

/*
stepSymbol advances one symbol's manifold: its Features, standardized
causally, settled against the previous epoch's vector as the supervised
target. A symbol whose feature schema changed since it learned sits the
round out rather than corrupting a learned manifold with a different input
shape.
*/
func (solver *Solver) stepSymbol(metrics map[string]map[string]map[string]float64, name string) map[string]float64 {
	if name == "" {
		return nil
	}

	if err := solver.Error(); err != nil {
		return nil
	}

	input, informative := solver.extractFeatures(name, metrics)
	if len(input) == 0 {
		return nil
	}

	// An unready vector is not a measurement: settling on it drives the latent
	// state to the origin and turns an absent observation into a structural
	// zero, so the reading says "not ready" and waits.
	if !informative {
		return nil
	}

	previous, hadPrevious := solver.takePending(name)

	manifold := solver.manifold(name, len(input))
	if manifold == nil {
		return nil
	}

	// The supervised target is the previous epoch's vector: the head learns
	// the one-step transition only once the future has actually arrived.
	target := []float64(nil)
	if hadPrevious && len(previous) == len(input) {
		target = previous
	}

	hadTarget := len(target) > 0

	if _, err := manifold.SettleFromBatchOptions(input, target, true, false); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("resonance: %s settle failed", name),
			err,
		))

		return nil
	}

	solver.pending.Store(name, append([]float64(nil), input...))

	return solver.publish(manifold, hadTarget)
}

/*
extractFeatures reads one symbol's feature vector off envelope Metrics: each
pinned Feature picked by source/label (aggregate scope uses the "aggregate"
symbol key when present), then standardized against that symbol's history.

informative reports whether at least one feature is an actual measurement.
A zero reading and an absent reading are not the same thing: during warm-up
the standardizer answers zero without readiness, while a ready observation
at the learned mean is legitimately zero. Settling on an all-unready vector
would teach the manifold that nothing moved.
*/
func (solver *Solver) extractFeatures(
	name string,
	metrics map[string]map[string]map[string]float64,
) ([]float64, bool) {
	readings := make(map[string]float64, len(Features))

	for _, feature := range Features {
		if feature.Source == "broker" {
			if value, ok := solver.brokerReading(name, feature.Label); ok {
				readings[featureReadingKey(feature)] = value
			}

			continue
		}

		scopeKey := name
		if feature.Scope == "aggregate" {
			scopeKey = "aggregate"
		}

		sources := metrics[scopeKey]
		if sources == nil {
			continue
		}

		labels := sources[feature.Source]
		if labels == nil {
			continue
		}

		if value, ok := labels[feature.Label]; ok {
			readings[featureReadingKey(feature)] = value
		}
	}

	if len(readings) == 0 {
		return nil, false
	}

	scorer, _ := solver.standardizers.LoadOrStore(name, newFeatureScorer())
	input, informative := scorer.(*featureScorer).Step(readings)

	return input, informative
}

/*
featureScorer standardizes one symbol's features causally: each sample is
scored against prior moments, never against the moments it is about to
contribute.
*/
type featureScorer struct {
	standardizers []*adaptive.Standardizer
	held          []float64
}

func newFeatureScorer() *featureScorer {
	scorer := &featureScorer{
		standardizers: make([]*adaptive.Standardizer, len(Features)),
		held:          make([]float64, len(Features)),
	}

	for i := range scorer.standardizers {
		scorer.standardizers[i] = adaptive.NewStandardizer()
	}

	return scorer
}

/*
Step returns the standardized vector in Feature order and reports whether any
feature carries a real measurement. A source the tick does not carry holds
the last standardized value, zero until its first arrival.
*/
func featureReadingKey(feature Feature) string {
	return feature.Source + "/" + feature.Label
}

func (scorer *featureScorer) Step(readings map[string]float64) ([]float64, bool) {
	input := make([]float64, len(Features))
	informative := false

	for i, feature := range Features {
		reading, ok := readings[featureReadingKey(feature)]
		if !ok {
			// Absent this round: hold the last standardized value without
			// feeding the standardizer a synthetic sample.
			input[i] = scorer.held[i]
			continue
		}

		output, err := scorer.standardizers[i].Measure(reading)
		if err != nil {
			continue
		}

		input[i] = output.Value
		scorer.held[i] = output.Value

		if output.Ready {
			informative = true
		}
	}

	return input, informative
}

/*
manifold returns symbol's predictive coder, building it on first use at the
configured architecture and pace.
*/
func (solver *Solver) manifold(name string, featureDim int) *learning.ResonanceManifold {
	loaded, _ := solver.manifolds.LoadOrStore(name, &sync.Map{})
	bucket := loaded.(*sync.Map)

	if prior, ok := bucket.Load("feature_dim"); ok && prior.(int) != featureDim {
		bucket.Delete("manifold")
		solver.pending.Delete(name)
	}

	if held, ok := bucket.Load("manifold"); ok {
		return held.(*learning.ResonanceManifold)
	}

	arch := coderArch(featureDim)

	// The task head is supervised on the same vector the coder observes, so
	// the target dimension equals the input dimension.
	manifold := learning.NewResonanceManifold(arch, featureDim, solver.pace)
	if manifold == nil {
		solver.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: unable to build manifold for %s", name),
			nil,
		))

		return nil
	}

	bucket.Store("feature_dim", featureDim)
	bucket.Store("manifold", manifold)
	return manifold
}

/*
coderArch is the configured predictive architecture: input -> latent ->
input, with the latent layer widened to hold the reconstruction.
*/
func coderArch(featureDim int) []int {
	layers := 3

	if configured := system.Cfg.Resonance.Layers; configured >= 2 {
		layers = configured
	}

	arch := make([]int, layers)
	arch[0] = featureDim

	for i := 1; i < layers; i++ {
		arch[i] = featureDim * 2
	}

	arch[layers-1] = featureDim
	return arch
}

/*
publish builds the scalar reading for one settled symbol.
*/
func (solver *Solver) publish(manifold *learning.ResonanceManifold, hadTarget bool) map[string]float64 {
	layers, surprise, energy := manifold.WireSnapshot()
	latent := manifold.LatentState()

	reading := map[string]float64{
		"surprise": surprise,
		"energy":   energy,
		"alpha":    solver.pace,
	}

	if skill, ok := manifold.TaskSkill(); ok {
		reading["task_skill"] = skill
	}

	if precision, ok := manifold.TaskPrecision(); ok {
		reading["task_relative_precision"] = precision
	}

	// The top layer's only residual is the temporal one; publishing it makes
	// the memory mismatch visible next to the sensory mismatch.
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

	// The forward curve is the one-step task reading: the supervised head is
	// trained on the next observation's vector, so its reading is that next
	// vector, not a price return. Publish it as the coder's own forecast.
	curve := manifold.TaskPrediction()

	for i, val := range curve {
		reading[fmt.Sprintf("forward_curve_%d", i)] = val
	}

	// The strategy gate reads supported_horizon/calibrated to decide whether
	// the head has learned anything yet. A head with no resolved sample has no
	// supported reach; one resolved sample supports exactly the one-step
	// transition it was trained on.
	supported := 0.0

	if hadTarget && len(curve) > 0 {
		supported = float64(len(curve))
	}

	reading["supported_horizon"] = supported

	if _, ok := manifold.TaskSkill(); ok {
		reading["calibrated"] = 1
	} else {
		reading["calibrated"] = 0
	}

	return reading
}

/*
takePending returns and replaces the feature vector issued at the previous
epoch for symbol.
*/
func (solver *Solver) takePending(name string) ([]float64, bool) {
	held, ok := solver.pending.Load(name)
	if !ok {
		return nil, false
	}

	previous, ok := held.([]float64)
	return previous, ok
}

/*
symbolsOnEnvelope lists every symbol the envelope carries, from trades and
from Metrics keys (except board "aggregate"), sorted for a stable pass.
*/
func symbolsOnEnvelope(envelope *runtime.Envelope, metrics map[string]map[string]map[string]float64) []string {
	seen := map[string]struct{}{}

	if envelope != nil && envelope.Trade != nil {
		for _, trade := range envelope.Trade.Data {
			if trade == nil || trade.Symbol == "" {
				continue
			}

			seen[trade.Symbol] = struct{}{}
		}
	}

	for symbol := range metrics {
		if symbol == "" || symbol == "aggregate" {
			continue
		}

		seen[symbol] = struct{}{}
	}

	out := make([]string, 0, len(seen))

	for symbol := range seen {
		out = append(out, symbol)
	}

	slices.Sort(out)
	return out
}

/*
brokerReading resolves one broker oracle feature for symbol. Veto dims are
positive when the desk gate fails; approve dims carry the raw strength.
*/
func (solver *Solver) brokerReading(symbol, label string) (float64, bool) {
	if solver == nil || solver.price == nil || symbol == "" || system.Cfg == nil || system.Cfg.Risk == nil {
		return 0, false
	}

	price := solver.price
	risk := system.Cfg.Risk

	switch label {
	case "volume_approve", "volume_veto":
		volume := price.VolumeUSD(symbol)
		floor := risk.MinMoverVolumeUSD

		if label == "volume_approve" {
			return volume, true
		}

		if floor <= 0 {
			return 0, true
		}

		return maxFloat64(0, floor-volume) / floor, true

	case "day_approve", "day_veto":
		change := price.ChangePct(symbol)
		floor := risk.MinMoverChangePct

		if label == "day_approve" {
			return change, true
		}

		if floor <= 0 {
			return 0, true
		}

		return maxFloat64(0, floor-change) / floor, true

	case "spread_approve", "spread_veto":
		spread, ok := price.SpreadPct(symbol)
		if !ok || spread == nil {
			return 0, false
		}

		pct := spread.MustFloat64()
		cap := risk.MaxEntrySpreadPct

		if label == "spread_veto" {
			return pct, true
		}

		if cap <= 0 {
			return 0, true
		}

		return maxFloat64(0, cap-pct) / cap, true

	case "book_approve", "book_veto":
		intact := price.BookIntact(symbol)

		if label == "book_approve" {
			if ratio, ok := price.BackingRatio(symbol); ok {
				return ratio, true
			}

			if intact {
				return 1, true
			}

			return 0, true
		}

		if intact {
			return 0, true
		}

		return 1, true
	}

	return 0, false
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}

	return b
}

/*
Reset clears learned temporal and precision state, and the per-symbol feature
moments and pending vectors, so a fresh tape starts honestly rather than
carrying a schema it no longer sees.
*/
func (solver *Solver) Reset(resetPrecision bool) {
	solver.manifolds.Range(func(_, held any) bool {
		if bucket, ok := held.(*sync.Map); ok {
			if manifold, ok := bucket.Load("manifold"); ok {
				manifold.(*learning.ResonanceManifold).ResetState(resetPrecision)
			}
		}
		return true
	})

	solver.standardizers.Clear()
	solver.pending.Clear()
}

/*
Close drops every manifold.
*/
func (solver *Solver) Close() error {
	solver.manifolds.Clear()
	solver.standardizers.Clear()
	solver.pending.Clear()
	return nil
}
