/*
Package advisor hosts SYMM's descriptive context layer: a single Advisor type
that composes already-produced Measurements into bounded per-symbol resident
state through a caller-supplied nomagique.Number pipeline, and emits
Perspectives — current descriptive context that is never a gate, a score, or a
trade instruction.

The contract every Advisor obeys:

 1. subscribe to a typed measurement stream;
 2. consume each event exactly once;
 3. mutate bounded resident state through the Number pipeline;
 4. emit a Perspective;
 5. retain no unbounded event backlog;
 6. never reconstruct a world snapshot to process one event.

Advisors compose existing Measurements rather than re-deriving raw signals, and
answer "what context is relevant now?" — never "what should be done?".

There is exactly one Advisor Go type. Every concrete advisor family (liquidity,
morphology, coordination, ...) is one Advisor instance constructed with its own
nomagique.Number pipeline and its own set of MetricBindings; the pipeline, not
the type, carries the family's mathematics.
*/
package advisor

import (
	"time"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
MetricBinding declares one measurement fact a composed Advisor's pipeline
consumes: the producing Source and the metric name exactly as the
Measurement.Metrics map keys it, plus the series Prefix that names the
interned Frame slots the value is projected into. The Source disambiguates a
metric name emitted by more than one signal. Prefix namespaces the binding's
value, event time, and derived estimator slots so several bindings can compose
into the same per-symbol Frame without collision.

Series is the resolved slot table for Prefix. Baseline, ZScore, and Velocity
name the same prefix's temporal-context estimator slots (statistic.Baseline /
statistic.ZScore / statistic.Velocity), resolved once here so a reading
pipeline built from those primitives can be projected back into a Perspective
without re-deriving the naming convention.
*/
type MetricBinding struct {
	Source   string
	Metric   string
	Prefix   string
	Series   temporal.Series
	Baseline nmtypes.Symbol
	ZScore   nmtypes.Symbol
	Velocity nmtypes.Symbol
}

/*
NewMetricBinding constructs one binding, interning its series prefix and
temporal-context estimator slots once at wiring time. The prefix should be
unique per composed metric within an Advisor so its Frame slots do not
collide with any other bound metric.
*/
func NewMetricBinding(source, metric, seriesPrefix string) MetricBinding {
	return MetricBinding{
		Source:   source,
		Metric:   metric,
		Prefix:   seriesPrefix,
		Series:   temporal.NewSeries(seriesPrefix),
		Baseline: nmtypes.MustIntern(temporal.JoinPrefix(seriesPrefix, "baseline/value")),
		ZScore:   nmtypes.MustIntern(temporal.JoinPrefix(seriesPrefix, "z/value")),
		Velocity: nmtypes.MustIntern(temporal.JoinPrefix(seriesPrefix, "velocity/delta")),
	}
}

/*
Advisor is the single context-producer type. It owns one nomagique.Number
pipeline — supplied at construction, never assumed — keyed by the logical
subject (the symbol) so every composed metric for that subject folds into the
same committed Number state, and projects the declared measurement metrics
into that pipeline through their bindings.
*/
type Advisor struct {
	name     string
	kind     types.PerspectiveKind
	number   *nomagique.Number[string]
	bindings []MetricBinding

	ObserveModule func(string, time.Duration)
}

/*
NewAdvisor constructs one Advisor over a caller-supplied pipeline. The pipeline
carries the family's mathematics (temporal, cross-sectional, relational,
whatever the family requires); Advisor only hosts it. Every binding declares a
measurement metric the pipeline composes; bindings and pipeline are built once
here, never per event.
*/
func NewAdvisor(
	name string,
	kind types.PerspectiveKind,
	pipeline nmtypes.Primitive,
	bindings ...MetricBinding,
) *Advisor {
	return &Advisor{
		name:     name,
		kind:     kind,
		number:   nomagique.NewNumber[string](pipeline),
		bindings: bindings,
	}
}

func (advisor *Advisor) Name() string { return advisor.name }

func (advisor *Advisor) Error() error { return nil }

/*
Close is a no-op: the advisor owns no goroutines and no external resources.
*/
func (advisor *Advisor) Close() error { return nil }

/*
Step folds one projected Measurement's relevant facts into the symbol's
composed resident state and returns the current Perspective for that symbol.
A nil, errored, or unlabeled measurement, or one with no binding relevant to
this Advisor, produces no context: events move, state stays.
*/
func (advisor *Advisor) Step(measurement *data.Measurement[float64]) *types.Perspective {
	if advisor == nil || measurement == nil || measurement.Err != nil || measurement.Label == "" {
		return nil
	}

	started := time.Now()

	frame := nmtypes.Frame{}
	relevant := false

	for _, binding := range advisor.bindings {
		if measurement.Source != binding.Source {
			continue
		}

		metric, found := measurement.Metrics[binding.Metric]

		if !found {
			continue
		}

		frame.Put(binding.Series.ValueSymbol, metric.Raw)
		frame.Put(binding.Series.SecSymbol, float64(measurement.At.Unix()))
		frame.Put(binding.Series.NsecSymbol, float64(measurement.At.Nanosecond()))
		relevant = true
	}

	if !relevant {
		return nil
	}

	state := advisor.number.Step(measurement.Label, frame)

	perspective := advisor.project(measurement.Label, measurement.At, state)

	if advisor.ObserveModule != nil {
		advisor.ObserveModule(advisor.name, time.Since(started))
	}

	return perspective
}

/*
project reads the symbol's committed pipeline state and materializes the
Perspective. A bound metric whose derived slots are not all present contributes
an explicitly not-ready reading: its absence is carried by Ready, never by a
fabricated zero.
*/
func (advisor *Advisor) project(
	symbol string,
	at time.Time,
	state nmtypes.Frame,
) *types.Perspective {
	perspective := &types.Perspective{
		Symbol: symbol,
		Kind:   advisor.kind,
		At:     at,
	}

	count := 0

	for _, binding := range advisor.bindings {
		if count >= types.PerspectiveMetricCapacity {
			break
		}

		value, hasValue := state.Get(binding.Series.ValueSymbol)
		baseline, hasBaseline := state.Get(binding.Baseline)
		zScore, hasZScore := state.Get(binding.ZScore)
		velocity, hasVelocity := state.Get(binding.Velocity)

		perspective.Readings[count] = types.MetricReading{
			Metric:   binding.Series.ValueSymbol,
			Value:    value,
			Baseline: baseline,
			ZScore:   zScore,
			Velocity: velocity,
			Ready:    hasValue && hasBaseline && hasZScore && hasVelocity,
		}

		count++
	}

	perspective.Count = count

	return perspective
}
