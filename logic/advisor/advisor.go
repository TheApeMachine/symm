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
	"fmt"
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

Series is the resolved slot table for Prefix.

Fresh names a marker slot Step populates only when this specific call's
Measurement actually carried the binding's metric. A pipeline branch built for
this binding must gate its own advance on Fresh: Number.Step merges the
previous committed Frame under the incoming one before running the pipeline,
so without a marker scoped to "this call's own input" every branch would see
the binding's value and event time as still present (retained from an earlier,
unrelated event) and would advance again on every other binding's Measurement.
The pipeline must also ensure Fresh never survives into what gets committed,
or it would read as fresh again on every later call regardless of what that
call delivered.

Maturity, SNR, and SNRDefined name where Step projects the source
Measurement's own quality facts for this metric, so a composed reading can
carry that provenance forward instead of discarding it.
*/
type MetricBinding struct {
	Source     string
	Metric     string
	Prefix     string
	Series     temporal.Series
	Fresh      nmtypes.Symbol
	Maturity   nmtypes.Symbol
	SNR        nmtypes.Symbol
	SNRDefined nmtypes.Symbol
}

/*
NewMetricBinding constructs one binding, interning its series prefix and
quality-provenance slots once at wiring time. The prefix should be unique per
composed metric within an Advisor so its Frame slots do not collide with any
other bound metric.
*/
func NewMetricBinding(source, metric, seriesPrefix string) MetricBinding {
	return MetricBinding{
		Source:     source,
		Metric:     metric,
		Prefix:     seriesPrefix,
		Series:     temporal.NewSeries(seriesPrefix),
		Fresh:      nmtypes.MustIntern(temporal.JoinPrefix(seriesPrefix, "advisor/fresh")),
		Maturity:   nmtypes.MustIntern(temporal.JoinPrefix(seriesPrefix, "advisor/maturity")),
		SNR:        nmtypes.MustIntern(temporal.JoinPrefix(seriesPrefix, "advisor/snr")),
		SNRDefined: nmtypes.MustIntern(temporal.JoinPrefix(seriesPrefix, "advisor/snr_defined")),
	}
}

/*
Output declares one named fact a pipeline emits for one composed metric: the
interned Slot a reading's value is read back from, and the Metric identity of
the composed metric it belongs to (for quality-provenance lookup, since
Maturity/SNR/SNRDefined are tracked per bound metric, not per output slot). A
pipeline built from temporal-context primitives declares four outputs per
bound metric (its value, baseline, z-score, and velocity); a different family's
pipeline declares whatever named facts its own mathematics produces. Advisor
never assumes a fixed output shape — it only walks whatever Outputs its
pipeline declared.
*/
type Output struct {
	Slot   nmtypes.Symbol
	Metric MetricBinding
}

/*
Advisor is the single context-producer type. It owns one nomagique.Number
pipeline — supplied at construction, never assumed — keyed by the logical
subject (the symbol) so every composed metric for that subject folds into the
same committed Number state. It ingests measurement facts through its
MetricBindings and projects a Perspective by walking its declared Outputs;
Advisor assumes nothing about what mathematics the pipeline performs or how
many named facts it produces per composed metric.
*/
type Advisor struct {
	name     string
	kind     types.PerspectiveKind
	number   *nomagique.Number[string]
	bindings []MetricBinding
	outputs  []Output

	ObserveModule func(string, time.Duration)
}

/*
NewAdvisor constructs one Advisor over a caller-supplied pipeline. The pipeline
carries the family's mathematics (temporal, cross-sectional, relational,
whatever the family requires); Advisor only hosts it. Every binding declares a
measurement metric the pipeline ingests; every output declares one named fact
the pipeline emits back. Bindings, outputs, and pipeline are built once here,
never per event.

NewAdvisor panics if outputs exceeds PerspectiveMetricCapacity: this is a
wiring-time structural mismatch between a pipeline's declared output count and
the fixed-size Perspective payload, not a runtime condition to degrade
through — silently truncating a future, wider pipeline's Outputs would drop
real readings without any signal that they were dropped.
*/
func NewAdvisor(
	name string,
	kind types.PerspectiveKind,
	pipeline nmtypes.Primitive,
	bindings []MetricBinding,
	outputs []Output,
) *Advisor {
	if len(outputs) > types.PerspectiveMetricCapacity {
		panic(fmt.Sprintf(
			"advisor: %d declared outputs exceed PerspectiveMetricCapacity (%d)",
			len(outputs), types.PerspectiveMetricCapacity,
		))
	}

	return &Advisor{
		name:     name,
		kind:     kind,
		number:   nomagique.NewNumber[string](pipeline),
		bindings: bindings,
		outputs:  outputs,
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

Only the bindings this specific Measurement actually carries are marked Fresh,
so the pipeline advances exactly the streams this event contributed to and
leaves every other bound metric's resident state untouched — Number.Step
merges the previous committed Frame under the incoming one before running the
pipeline, so without that marker a pipeline branch could not tell a fact this
event delivered from one merely retained from an earlier, unrelated event.
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
		frame.Put(binding.Fresh, 1)
		frame.Put(binding.Maturity, measurement.Maturity)
		frame.Put(binding.SNR, measurement.SNR)

		snrDefined := 0.0

		if measurement.SNRDefined {
			snrDefined = 1
		}

		frame.Put(binding.SNRDefined, snrDefined)
		relevant = true
	}

	if !relevant {
		return nil
	}

	output := advisor.number.Step(measurement.Label, frame)

	state, _ := advisor.number.Project(measurement.Label)
	perspective := advisor.project(measurement.Label, measurement.At, state)
	perspective.Err = output.Err

	if advisor.ObserveModule != nil {
		advisor.ObserveModule(advisor.name, time.Since(started))
	}

	return perspective
}

/*
project reads the symbol's committed pipeline state and materializes the
Perspective by walking the Advisor's declared Outputs — never by assuming a
fixed shape of what the pipeline produces per composed metric. An output whose
slot is not yet populated contributes an explicitly undefined reading: its
absence is carried by Defined, never by a fabricated zero. NewAdvisor already
rejects more outputs than PerspectiveMetricCapacity, so every output declared
here has a slot.
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

	for _, output := range advisor.outputs {
		value, defined := state.Get(output.Slot)
		maturity, _ := state.Get(output.Metric.Maturity)
		snr, _ := state.Get(output.Metric.SNR)
		snrDefined, _ := state.Get(output.Metric.SNRDefined)

		perspective.Readings[count] = types.MetricReading{
			Metric:     output.Slot,
			Value:      value,
			Defined:    defined,
			Maturity:   maturity,
			SNR:        snr,
			SNRDefined: snrDefined != 0,
		}

		count++
	}

	perspective.Count = count

	return perspective
}
