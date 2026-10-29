package resonance

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Solver runs one predictive-coding manifold per symbol over that symbol's
signal features (Features), and publishes the settled reading on the tick
under resonance/<symbol>/… for the strategy stage and the dashboard to read.

The manifold is the CPU predictive coder in nomagique/learning: it settles a
latent state that explains the current observation (its residual is Surprise),
carries a temporal prior across ticks, and carries a supervised task head
whose target is the next observation's feature vector.
*/
type Solver struct {
	*runtime.System
	manifolds *sync.Map
	pace      float64

	// standardizers hold each symbol's per-feature moments, so a feature is
	// scored against its own history rather than the raw feed.
	standardizers *sync.Map

	// pending holds, per symbol, the feature vector issued at the previous
	// analysis epoch. Its target is the current vector: the supervised head
	// trains on the future once it has actually arrived.
	pending *sync.Map
}

/*
Feature is one signal metric the solver reads: the label a source writes
(the same labels the grid pins, nomagique/store/grid_pins.go).
*/
type Feature struct {
	Source string
	Label  string
}

/*
Features are the solver's inputs, one headline metric per signal source, in
coder order. A source the tick does not carry (signals arrive with trades, so
not every tick carries every source) holds its last standardized value, zero
until its first arrival.
*/
var Features = []Feature{
	{"correlation", "cohort_covariance_score"},
	{"leadlag", "covariance_score_gain_median"},
	{"liquidity", "relative_spread"},
	{"sentiment", "breadth"},
	{"cvd", "signed_net_fraction"},
	{"depthflow", "book_imbalance"},
	{"morphology", "morphology_change"},
	{"hawkes", "branching_spectral_radius"},
	{"pumpdump", "spread_ratio"},
	{"toxicity", "retreat_fraction:bid"},
}

/*
ReadingOrigin and ReadingRole identify a Step's reading Value; its Scope is
the symbol, its Epoch, Tick and Timestamp those of the tick.
*/
const (
	ReadingOrigin = "resonance"
	ReadingRole   = "reading"
)

// paceUpper is the manifold's fastest configured pace; a slower resting pace
// only widens the warm-up band of its error average.
const paceUpper = 0.150

/*
NewSolver returns a solver stepping at pace; a pace that is not positive is
an error.
*/
func NewSolver(ctx context.Context, pace ...float64) *Solver {
	if len(pace) > 0 {
		if !(pace[0] > 0) || pace[0] > paceUpper {
			errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf(
				"resonance: pace must be in (0, %g], got %g", paceUpper, pace[0],
			), nil))

			return nil
		}
	}

	solver := &Solver{
		System:        runtime.NewSystem(ctx, "resonance"),
		manifolds:     &sync.Map{},
		pace:          paceValue(pace),
		standardizers: &sync.Map{},
		pending:       &sync.Map{},
	}

	solver.Transition(runtime.READY)
	return solver
}

func paceValue(pace []float64) float64 {
	if len(pace) > 0 && pace[0] > 0 {
		return pace[0]
	}

	if system := system.Cfg.Resonance.LearningRate; system > 0 {
		return system
	}

	return 0.03
}

/*
Step advances every symbol on the workspace tick through its predictive
coder and attaches scalar readings on the tick under resonance/<symbol>/….
Symbols run concurrently (signal.RunSymbols); per-symbol manifold state is
sharded and tick.Append is synchronized.
*/
func (solver *Solver) Step(tick *data.Value) *data.Value {
	if tick == nil {
		errnie.Error(errnie.Err(errnie.Validation, "resonance: Step needs a tick", nil))
		return tick
	}

	if err := solver.Error(); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "resonance: solver failed earlier", err))
		return tick
	}

	if err := signal.RunSymbols(solver.Context(), symbolsOnTick(tick), func(symbol string) {
		reading := solver.stepSymbol(tick, symbol)

		if reading != nil {
			attachReading(tick, symbol, reading)
		}
	}); err != nil {
		solver.Error(err)
	}

	return tick
}

/*
stepSymbol advances one symbol's manifold: its Features, standardized
causally, settled against the previous epoch's vector as the supervised
target. A symbol whose feature schema changed since it learned sits the
round out rather than corrupting a learned manifold with a different input
shape.
*/
func (solver *Solver) stepSymbol(tick *data.Value, name string) *data.Value {
	if tick == nil || name == "" {
		return nil
	}

	if err := solver.Error(); err != nil {
		return nil
	}

	input, informative := solver.extractFeatures(name, tick)
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

	if _, err := manifold.SettleFromBatchOptions(input, target, true, false); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("resonance: %s settle failed", name),
			err,
		))

		return nil
	}

	solver.pending.Store(name, append([]float64(nil), input...))

	reading := solver.publish(name, manifold, len(input))

	if ui.UI != nil && ui.UI.Active() {
		io.Copy(ui.UI, reading)
	}

	return reading
}

/*
extractFeatures reads one symbol's feature vector off the tick: every
signal layer grouped by source, each pinned Feature picked by its label,
then standardized against that symbol's own history.

informative reports whether at least one feature is an actual measurement.
A zero reading and an absent reading are not the same thing: during warm-up
the standardizer answers zero without readiness, while a ready observation
at the learned mean is legitimately zero. Settling on an all-unready vector
would teach the manifold that nothing moved.
*/
func (solver *Solver) extractFeatures(name string, tick *data.Value) ([]float64, bool) {
	type sig struct {
		labels []string
		values []float64
	}

	// Symbols are written as two path segments (e.g. "XBT/USD") and labels
	// follow them.
	bySource := map[string]map[string]sig{}

	for _, source := range types.SignalSourceStrings {
		for _, layer := range tick.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")
			if len(parts) < 4 {
				continue
			}

			symbol := parts[1] + "/" + parts[2]
			if symbol != name {
				continue
			}

			var values []float64
			if err := layer.Decode(&values); err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"[resonance.Step] "+layer.Key+" does not decode",
					err,
				))
				continue
			}

			if bySource[source] == nil {
				bySource[source] = make(map[string]sig)
			}

			bySource[source][symbol] = sig{
				labels: parts[3:],
				values: values,
			}
		}
	}

	sources := make(map[string]float64, len(Features))

	for _, feature := range Features {
		symbolSigs, ok := bySource[feature.Source]
		if !ok {
			continue
		}

		s, ok := symbolSigs[name]
		if !ok {
			continue
		}

		for idx, label := range s.labels {
			if label == feature.Label && idx < len(s.values) {
				sources[feature.Source] = s.values[idx]
				break
			}
		}
	}

	if len(sources) == 0 {
		return nil, false
	}

	scorer, _ := solver.standardizers.LoadOrStore(name, newFeatureScorer())
	input, informative := scorer.(*featureScorer).Step(sources)

	return input, informative
}

/*
featureScorer standardizes one symbol's features causally: each sample is
scored against prior moments, never against the moments it is about to
contribute.
*/
type featureScorer struct {
	standardizers []*adaptive.Standardizer
}

func newFeatureScorer() *featureScorer {
	scorer := &featureScorer{
		standardizers: make([]*adaptive.Standardizer, len(Features)),
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
func (scorer *featureScorer) Step(sources map[string]float64) ([]float64, bool) {
	input := make([]float64, len(Features))
	informative := false

	for i, feature := range Features {
		reading, ok := sources[feature.Source]
		if !ok {
			// Absent this round: hold the last standardized value.
			input[i] = scorer.standardizers[i].Standardize(reading)
			continue
		}

		output, err := scorer.standardizers[i].Measure(reading)
		if err != nil {
			continue
		}

		input[i] = output.Value

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
publish builds the reading Value for one settled symbol: one scalar layer per
metric, addressed so the strategy stage and the dashboard can read
resonance/<symbol>/<metric> off the tick.
*/
func (solver *Solver) publish(name string, manifold *learning.ResonanceManifold, featureDim int) *data.Value {
	reading := data.NewValue(ReadingOrigin, ReadingRole, name)

	layers, surprise, energy := manifold.WireSnapshot()
	latent := manifold.LatentState()

	reading.Append([]float64{surprise}, "surprise")
	reading.Append([]float64{energy}, "energy")
	reading.Append([]float64{solver.pace}, "alpha")

	if skill, ok := manifold.TaskSkill(); ok {
		reading.Append([]float64{skill}, "task_skill")
	}

	if precision, ok := manifold.TaskPrecision(); ok {
		reading.Append([]float64{precision}, "task_relative_precision")
	}

	// The top layer's only residual is the temporal one; publishing it makes
	// the memory mismatch visible next to the sensory mismatch.
	if temporal, ok := manifold.TemporalError(); ok {
		reading.Append([]float64{temporal}, "temporal_error")
	}

	for i, val := range latent {
		reading.Append([]float64{val}, fmt.Sprintf("latent_%d", i))
	}

	for layer, wire := range layers {
		reading.Append([]float64{wire.ErrorNorm}, fmt.Sprintf("layer_%d_error", layer))

		if wire.Temporal {
			reading.Append([]float64{1}, fmt.Sprintf("layer_%d_temporal", layer))
		} else {
			reading.Append([]float64{0}, fmt.Sprintf("layer_%d_temporal", layer))
		}

		for j, val := range wire.State {
			reading.Append([]float64{val}, fmt.Sprintf("layer_%d_state_%d", layer, j))
		}

		for j, val := range wire.Prediction {
			reading.Append([]float64{val}, fmt.Sprintf("layer_%d_prediction_%d", layer, j))
		}
	}

	// The forward curve is the one-step task reading: the supervised head is
	// trained on the next observation's vector, so its reading is that next
	// vector, not a price return. Publish it as the coder's own forecast.
	curve := manifold.TaskPrediction()

	for i, val := range curve {
		reading.Append([]float64{val}, fmt.Sprintf("forward_curve_%d", i))
	}

	// The strategy gate reads supported_horizon/calibrated to decide whether
	// the head has learned anything yet. A head with no resolved sample has no
	// supported reach; one resolved sample supports exactly the one-step
	// transition it was trained on.
	supported := 0.0
	if _, hadPrevious := solver.pending.Load(name); hadPrevious && featureDim > 0 {
		supported = float64(len(curve))
	}

	reading.Append([]float64{supported}, "supported_horizon")

	if skill, ok := manifold.TaskSkill(); ok {
		reading.Append([]float64{1}, "calibrated")
		_ = skill
	} else {
		reading.Append([]float64{0}, "calibrated")
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
symbolsOnTick lists every symbol the tick carries, from trades and from any
signal source, sorted for a stable pass.
*/
func symbolsOnTick(tick *data.Value) []string {
	if tick == nil {
		return nil
	}

	seen := map[string]struct{}{}

	var trades []kraken.TradeData
	tick.Lookup(&trades, "spot:trade")

	for _, trade := range trades {
		if trade.Symbol == "" {
			continue
		}

		seen[trade.Symbol] = struct{}{}
	}

	for _, source := range types.SignalSourceStrings {
		for _, layer := range tick.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")

			if len(parts) < 4 {
				continue
			}

			seen[parts[1]+"/"+parts[2]] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))

	for symbol := range seen {
		out = append(out, symbol)
	}

	slices.Sort(out)
	return out
}

/*
attachReading copies a reading's scalar layers onto the workspace tick as
resonance/<symbol>/<metric>, the addressing the strategy stage and the
dashboard both read.
*/
func attachReading(tick *data.Value, symbol string, reading *data.Value) {
	if tick == nil || reading == nil || symbol == "" {
		return
	}

	scope := strings.ReplaceAll(symbol, "/", "-")

	for label, val := range floatMetrics(reading) {
		tick.Append([]float64{val}, "resonance", scope, label)
	}
}

/*
floatMetrics flattens a reading into label -> first scalar value.
*/
func floatMetrics(value *data.Value) map[string]float64 {
	out := map[string]float64{}

	if value == nil {
		return out
	}

	for _, layer := range value.LookupPrefix("") {
		rest := layer.Key

		if value.Prefix != "" && strings.HasPrefix(rest, value.Prefix+"/") {
			rest = strings.TrimPrefix(rest, value.Prefix+"/")
		}

		labels := strings.Split(rest, "/")

		var values []float64
		if err := layer.Decode(&values); err != nil {
			continue
		}

		for i, reading := range values {
			if i < len(labels) {
				out[labels[i]] = reading
			}
		}
	}

	return out
}

/*
Reset clears learned temporal and precision state, and the per-symbol feature
moments and pending vectors, so a fresh tape starts honestly rather than
carrying a schema it no longer sees.
*/
func (solver *Solver) Reset(resetPrecision bool) {
	if solver.manifolds != nil {
		solver.manifolds.Range(func(_, held any) bool {
			if bucket, ok := held.(*sync.Map); ok {
				if manifold, ok := bucket.Load("manifold"); ok {
					manifold.(*learning.ResonanceManifold).ResetState(resetPrecision)
				}
			}
			return true
		})
	}

	if solver.standardizers != nil {
		solver.standardizers.Clear()
	}

	if solver.pending != nil {
		solver.pending.Clear()
	}
}

/*
Close drops every manifold.
*/
func (solver *Solver) Close() error {
	if solver.manifolds != nil {
		solver.manifolds.Clear()
	}

	if solver.standardizers != nil {
		solver.standardizers.Clear()
	}

	if solver.pending != nil {
		solver.pending.Clear()
	}

	return nil
}
