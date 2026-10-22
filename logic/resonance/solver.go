package resonance

import (
	"context"
	"fmt"
	goruntime "runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Solver runs one predictive coder per symbol over that symbol's signal
features (Features), at the configured pace. Each Step reads one symbol of
one stored tick and returns the coder's reading as a Value; its "surprise"
is the coder's reconstruction error per feature (ReadSurprise).
*/
type Solver struct {
	*runtime.System
	detectors     *sync.Map
	standardizers *sync.Map
	references    *sync.Map
	steps         *sync.Map
	pace          float64

	// ObserveModule is an optional diagnostics hook reporting per-step coder
	// duration so the wiring diagram can profile the resonance stage like
	// every other pipeline node.
	ObserveModule func(string, time.Duration)
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
coder order. A source a tick carries for the symbol without its pinned label
is an error; a source the tick does not carry (signals arrive with trades,
so not every tick carries every source) holds its last standardized value,
zero until its first arrival.
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

/*
NewSolver returns a solver stepping at pace; a pace that is not positive is
an error.
*/
func NewSolver(ctx context.Context, pace float64) (*Solver, error) {
	if !(pace > 0) || pace > paceUpper {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"resonance: pace must be in (0, %g] (a coder's rate rests at pace and moves up to %g), got %g", paceUpper, paceUpper, pace,
		), nil)
	}

	solver := &Solver{
		System:        runtime.NewSystem(ctx, "resonance"),
		detectors:     &sync.Map{},
		standardizers: &sync.Map{},
		references:    &sync.Map{},
		steps:         &sync.Map{},
		pace:          pace,
	}

	solver.Transition(runtime.READY)
	return solver, nil
}

/*
Step advances symbol's predictive coder by one stored tick: its Features,
standardized causally, and the tick's touch midpoint as the coder's
reference (no reference when the tick recorded either side as absent).
*/
func (solver *Solver) Step(tick *data.Value) (*data.Value, error) {
	if tick == nil {
		return nil, errnie.Err(errnie.Validation, "resonance: Step needs a tick and one of its symbols", nil)
	}

	if err := solver.Error(); err != nil {
		return nil, errnie.Err(errnie.Internal, "resonance: solver failed earlier", err)
	}

	name := tick.Scope
	sources := make(map[string]float64, len(Features))

	// Group every signal layer on the tick by source and symbol, then pick the
	// value each feature's label carries for this symbol. Symbols are written
	// as two path segments (e.g. "XBT/USD") and labels follow them.
	type sig struct {
		labels []string
		values []float64
	}
	bySource := map[string]map[string]sig{}

	for _, source := range types.SignalSourceStrings {
		layers := tick.LookupPrefix(source)

		for _, layer := range layers {
			parts := strings.Split(layer.Key, "/")
			if len(parts) < 3 {
				continue
			}

			symbol := parts[1] + "/" + parts[2]
			if symbol != name {
				continue
			}

			var values []float64
			if err := layer.Decode(&values); err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"[resonance.Step] "+layer.Key+" does not decode",
					err,
				))
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

	features, err := solver.scorer(name).Step(sources)

	if err != nil {
		return nil, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf("resonance: %s of %s", name, tick.Prefix), err)
	}

	var midpoint float64

	// No price/book layer is carried on the tick Value in the current
	// refactor, so there is no touch midpoint to read. The coder steps with
	// midpoint 0 and no reference until a price source is plumbed in.

	loadedStep, _ := solver.steps.LoadOrStore(name, &atomic.Int64{})
	step := loadedStep.(*atomic.Int64).Add(1)

	coder, err := solver.coder(name, len(features))

	if err != nil {
		return nil, err
	}

	hasReference := 0.0

	if _, held := solver.references.Load(name); held {
		hasReference = 1
	}

	if midpoint > 0 {
		solver.references.Store(name, midpoint)
	}

	stepStarted := time.Now()
	out := transport.Read[[12][]float64](coder.Next(transport.NewValue([2][]float64{
		features,
		{midpoint, hasReference, float64(step), float64(tick.Timestamp) / 1e9},
	}).Next(nil)))

	if solver.ObserveModule != nil {
		solver.ObserveModule("resonance", time.Since(stepStarted))
	}

	if err := coder.Error(); err != nil {
		return nil, errnie.Err(errnie.Internal, fmt.Sprintf("resonance: coder step failed for %s at %s", name, tick.Prefix), err)
	}

	reading := data.NewValue(ReadingOrigin, ReadingRole, name)
	reading.Epoch, reading.Tick, reading.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp

	if _, err := publishReturns(reading, out, coderArch(len(features))); err != nil {
		return nil, errnie.Err(errnie.Internal, fmt.Sprintf("resonance: unable to publish coder reading for %s", name), err)
	}

	return reading, nil
}

/*
ReadSurprise is the surprise a Step reading carries.
*/
func ReadSurprise(reading *data.Value) (float64, error) {
	surprise, ok := floatMetrics(reading)["surprise"]

	if !ok {
		return 0, errnie.Err(errnie.UnprocessableContent, "resonance: reading carries no surprise", nil)
	}

	return surprise, nil
}

/*
paceUpper is the fastest learning rate a symbol's coder may move to (its
Pace's upper bound); the pace it rests at is the solver's.
*/
const paceUpper = 0.150

/*
Rates are the two timescales of every coder's Pace in a solver stepping at
pace: fast, the rate a coder moves up to when its error is badly ranked, and
slow, the pace it rests at. They are the coders' own learning timescales, so
averages of a coder's error at these rates track it as fast and as slowly as
it can learn.
*/
func Rates(pace float64) (fast, slow float64) {
	return paceUpper, pace
}

func coderArch(featureDim int) []int {
	return []int{featureDim, featureDim * 4, featureDim * 2, featureDim}
}

func (solver *Solver) coder(symbol string, featureDim int) (*learning.PredictiveCoder, error) {
	if loaded, found := solver.detectors.Load(symbol); found {
		return loaded.(*learning.PredictiveCoder), nil
	}

	pace := learning.NewPace(solver.pace, solver.pace*0.1, paceUpper, 0.1, 0.2, 256)
	target := learning.NewDirectionalTarget(0)
	coder := learning.NewPredictiveCoder(coderArch(featureDim), 8, target, pace, 0.05, true, learning.ReadoutAll).(*learning.PredictiveCoder)

	if err := coder.Error(); err != nil {
		return nil, errnie.Err(errnie.Internal, "resonance: predictive coder for "+symbol, err)
	}

	actual, _ := solver.detectors.LoadOrStore(symbol, coder)
	return actual.(*learning.PredictiveCoder), nil
}

func (solver *Solver) scorer(symbol string) *featureScorer {
	loaded, found := solver.standardizers.Load(symbol)

	if found {
		if s, ok := loaded.(*featureScorer); ok && s != nil {
			return s
		}
	}

	created := newFeatureScorer()
	actual, _ := solver.standardizers.LoadOrStore(symbol, created)
	return actual.(*featureScorer)
}

type featureScorer struct {
	isStepping   atomic.Bool
	moments      []core.Primitive
	residuals    []core.Primitive
	standardized []float64
}

func newFeatureScorer() *featureScorer {
	scorer := &featureScorer{
		moments:      make([]core.Primitive, len(Features)),
		residuals:    make([]core.Primitive, len(Features)),
		standardized: make([]float64, len(Features)),
	}

	for index := range scorer.moments {
		scorer.moments[index] = statistic.NewEstimator()
		scorer.residuals[index] = statistic.NewCausalResidual()
	}

	return scorer
}

/*
Step standardizes each Feature the tick carries for the symbol (a causal
residual z-score over its running moments) and returns every Feature's
latest standardized value, in Features order.
*/
func (scorer *featureScorer) Step(sources map[string]float64) ([]float64, error) {
	for !scorer.isStepping.CompareAndSwap(false, true) {
		goruntime.Gosched()
	}
	defer scorer.isStepping.Store(false)

	for source := range sources {
		if !slices.ContainsFunc(Features, func(feature Feature) bool { return feature.Source == source }) {
			return nil, errnie.Err(errnie.Validation, "resonance: signal source "+source+" has no pinned feature", nil)
		}
	}

	for index, feature := range Features {
		val, carried := sources[feature.Source]

		if !carried {
			continue
		}

		var reading *statistic.MomentReading

		for out := range scorer.moments[index].Next(transport.NewValue(val).Next(nil)) {
			reading = (*statistic.MomentReading)(out)
		}

		if err := scorer.moments[index].Error(); err != nil || reading == nil {
			return nil, errnie.Err(errnie.Internal, fmt.Sprintf("resonance: moments of %s/%s", feature.Source, feature.Label), err)
		}

		var residual *statistic.CausalResidualResult

		for out := range scorer.residuals[index].Next(transport.NewValue(*reading).Next(nil)) {
			residual = (*statistic.CausalResidualResult)(out)
		}

		if err := scorer.residuals[index].Error(); err != nil || residual == nil {
			return nil, errnie.Err(errnie.Internal, fmt.Sprintf("resonance: residual of %s/%s", feature.Source, feature.Label), err)
		}

		scorer.standardized[index] = residual.ZScore
	}

	return slices.Clone(scorer.standardized), nil
}

func publishReturns(prior *data.Value, out [12][]float64, arch []int) (*data.Value, error) {
	if len(out[7]) > 0 {
		offset := 0
		for layer, rows := range arch {
			width := 2 + 2*rows
			if offset+width > len(out[7]) {
				return nil, fmt.Errorf("resonance: layer reading holds %d values, layer %d of arch %v needs %d", len(out[7]), layer, arch, offset+width)
			}
			offset += width
		}
		if offset != len(out[7]) {
			return nil, fmt.Errorf("resonance: layer reading holds %d values, arch %v accounts for %d", len(out[7]), arch, offset)
		}
	}

	// The summary row is {reconstruction, energy, ..., surprise (5), ...}.
	if len(out[0]) <= 5 {
		return nil, fmt.Errorf("resonance: coder summary holds %d values, energy and surprise need 6", len(out[0]))
	}

	prior.Append([]float64{out[0][1]}, "energy")
	prior.Append([]float64{out[0][5]}, "surprise")

	for i, val := range out[2] {
		prior.Append([]float64{val}, fmt.Sprintf("latent_%d", i))
	}

	if len(out[7]) > 0 {
		offset := 0
		for layer, rows := range arch {
			width := 2 + 2*rows
			block := out[7][offset : offset+width]
			offset += width

			prior.Append([]float64{block[0]}, fmt.Sprintf("layer_%d_error", layer))
			prior.Append([]float64{block[1]}, fmt.Sprintf("layer_%d_temporal", layer))

			for j := range rows {
				prior.Append([]float64{block[2+j]}, fmt.Sprintf("layer_%d_state_%d", layer, j))
				prior.Append([]float64{block[2+rows+j]}, fmt.Sprintf("layer_%d_prediction_%d", layer, j))
			}
		}
	}

	for i, val := range out[11] {
		prior.Append([]float64{val}, fmt.Sprintf("forward_curve_%d", i))
	}

	return prior, nil
}

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
