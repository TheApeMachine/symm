/*
Package advisor hosts SYMM's descriptive context layer: a single Advisor type
that composes already-produced Measurements into bounded per-symbol resident
state through a nomagique.Number pipeline, and emits Perspectives — current
descriptive context that is never a gate, a score, or a trade instruction.

The contract every Advisor obeys:

 1. subscribe to a typed measurement stream;
 2. consume each event exactly once;
 3. mutate bounded resident state through the Number pipeline;
 4. emit a Perspective;
 5. retain no unbounded event backlog;
 6. never reconstruct a world snapshot to process one event.

Advisors compose existing Measurements rather than re-deriving raw signals, and
answer "what context is relevant now?" — never "what should be done?".
*/
package advisor

import (
	"time"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
MetricBinding declares one measurement a composed Advisor feeds: the producing
Source and the metric name exactly as the Measurement.Metrics map keys it. The
Source disambiguates a metric name emitted by more than one signal.
*/
type MetricBinding struct {
	Source string
	Metric string
}

/*
advisorKey is the bounded resident-state identity of one composed metric for one
symbol. Each key owns one independent Number stream, so a symbol's composed
metrics evolve independently without slot collision.
*/
type advisorKey struct {
	symbol string
	source string
	metric string
}

/*
stage is the shared temporal-context pipeline each composed metric runs: retain
a bounded event-time window, then derive the adaptive baseline, the departure
z-score, and the first difference. The empty prefix uses the legacy generic
slots, so the per-metric key — not the slot namespace — keeps streams apart.
*/
func stage() nmtypes.Primitive {
	return nmtypes.Pipe(
		temporal.Window(""),
		statistic.ZScore(""),
		statistic.Baseline(""),
		statistic.Velocity(""),
	)
}

/*
Advisor is the single context-producer type. It owns one nomagique.Number
pipeline — the bounded per-symbol, per-metric resident state plus its derived
statistics — and composes the declared Measurement metrics through it.
*/
type Advisor struct {
	name    string
	kind    types.PerspectiveKind
	number  *nomagique.Number[advisorKey]
	metrics []MetricBinding

	sequence      uint64
	ObserveModule func(string, time.Duration)
}

/*
NewAdvisor constructs one composed-metric Advisor. Every binding declares a
measurement metric the advisor maintains temporal context for; the pipeline and
resident state are built once here, never per event.
*/
func NewAdvisor(name string, kind types.PerspectiveKind, metrics ...MetricBinding) *Advisor {
	return &Advisor{
		name:    name,
		kind:    kind,
		number:  nomagique.NewNumber[advisorKey](stage()),
		metrics: metrics,
	}
}

func (advisor *Advisor) Name() string { return advisor.name }

func (advisor *Advisor) Error() error { return nil }

/*
Close is a no-op: the advisor owns no goroutines and no external resources.
*/
func (advisor *Advisor) Close() error { return nil }

/*
Step folds one projected Measurement into the composed metrics' resident state
and returns the current Perspective for that symbol. A nil, errored, or
unlabeled measurement produces no context.
*/
func (advisor *Advisor) Step(measurement *data.Measurement[float64]) *types.Perspective {
	if advisor == nil || measurement == nil || measurement.Err != nil || measurement.Label == "" {
		return nil
	}

	started := time.Now()

	for _, binding := range advisor.metrics {
		if measurement.Source != binding.Source {
			continue
		}

		metric, found := measurement.Metrics[binding.Metric]

		if !found {
			continue
		}

		advisor.ingest(measurement.Label, binding, metric.Raw, measurement.At)
	}

	perspective := advisor.project(measurement.Label, measurement.At)

	if advisor.ObserveModule != nil {
		advisor.ObserveModule(advisor.name, time.Since(started))
	}

	return perspective
}

/*
ingest feeds one composed metric's current value and event time into its keyed
pipeline stream.
*/
func (advisor *Advisor) ingest(symbol string, binding MetricBinding, value float64, at time.Time) {
	frame := nmtypes.Frame{}
	frame.Put(temporal.DefaultSeries.ValueSymbol, value)
	frame.Put(temporal.SymbolUnixSec, float64(at.Unix()))
	frame.Put(temporal.SymbolUnixNsec, float64(at.Nanosecond()))

	advisor.number.Step(advisorKey{
		symbol: symbol,
		source: binding.Source,
		metric: binding.Metric,
	}, frame)
}

/*
project reads each composed metric's committed state and materializes the
Perspective. A metric whose derived slots are not all present contributes no
reading: its absence is carried by Count, never by a fabricated zero.
*/
func (advisor *Advisor) project(symbol string, at time.Time) *types.Perspective {
	perspective := &types.Perspective{
		Symbol:   symbol,
		Kind:     advisor.kind,
		At:       at,
		Sequence: advisor.sequence,
	}

	advisor.sequence++

	count := 0

	for _, binding := range advisor.metrics {
		if count >= types.PerspectiveMetricCapacity {
			break
		}

		frame, found := advisor.number.Project(advisorKey{
			symbol: symbol,
			source: binding.Source,
			metric: binding.Metric,
		})

		if !found {
			continue
		}

		value, hasValue := frame.Get(temporal.DefaultSeries.ValueSymbol)
		baseline, hasBaseline := frame.Get(statistic.SymbolBaselineValue)
		zScore, hasZScore := frame.Get(statistic.SymbolZScore)
		velocity, hasVelocity := frame.Get(statistic.SymbolVelocityDelta)

		perspective.Readings[count] = types.MetricReading{
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
