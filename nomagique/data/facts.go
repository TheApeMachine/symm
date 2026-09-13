package data

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
MetricGate classifies the arrival against one declared metric: the metric must
exist and hold a finite, non-negative value. It rewrites the validated metric
and stamps the support baseline on every arrival, so the measurement carries
this arrival's fact, never the prior one's. A failed classification sets the
measurement's error and still yields.
*/
type MetricGate struct {
	err    error
	label  string
	finite core.Primitive
}

func NewMetricGate(label string) core.Primitive {
	return &MetricGate{label: label, finite: logic.NewFinite()}
}

func (op *MetricGate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**Measurement[float64])(arriving)

			metric, holds := m.Metrics[op.label]

			if !holds {
				m.Err = fmt.Errorf("%w: metric gate requires %s", core.ErrDomain, op.label)

				if !yield(arriving) {
					return
				}

				continue
			}

			value := metric.Raw
			m.Metadata = map[string]float64{MetadataSupport: 0}

			finite := false

			for out := range op.finite.Next(transport.NewOne(unsafe.Pointer(&value)).Next(nil)) {
				finite = *(*bool)(out)
			}

			if err := op.finite.Error(); err != nil {
				m.Err = err

				if !yield(arriving) {
					return
				}

				continue
			}

			if !finite || value < 0 {
				m.Err = fmt.Errorf(
					"%w: metric gate requires a finite non-negative %s",
					core.ErrDomain, op.label,
				)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics[op.label] = metric.Write(value)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *MetricGate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = fmt.Errorf("%w: %s", err, op.label)
		}
	}

	return op.err
}

/*
CrossSectionFolds folds the arrival's declared metric into a cross-section
keyed by the measurement's label and writes the generic snapshot facts back
into the measurement where they are computed: sign counts, robust aggregates,
extremes, cohort ages, and each aggregate's causal view. An arrival that
produces no snapshot (the key's first observation) yields the measurement
unchanged.
*/
type CrossSectionFacts struct {
	err     error
	label   string
	section core.Primitive
}

func NewCrossSectionFacts(label string) core.Primitive {
	return &CrossSectionFacts{label: label, section: NewCrossSection()}
}

func (op *CrossSectionFacts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			input := SectionInput{
				Key:   m.Label,
				Value: m.Metrics[op.label].Raw,
				At:    m.At,
				Focal: m.Label,
			}

			for out := range op.section.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
				snapshot := *(*Snapshot)(out)

				if snapshot.At.IsZero() {
					continue
				}

				op.write(m, snapshot)
			}

			if err := op.section.Error(); err != nil {
				m.Err = err
				op.Error(err)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
write stamps one snapshot's generic facts into the measurement. Fact names are
the container's own vocabulary: changes, counts, robust aggregates, extremes,
ages, and causal views.
*/
func (op *CrossSectionFacts) write(m *Measurement[float64], snapshot Snapshot) {
	facts := []struct {
		label string
		value float64
		unit  Unit
	}{
		{"valid_member_count", float64(snapshot.Count), UnitCount},
		{"member_count", float64(snapshot.TotalMembers), UnitCount},
		{"excluded_member_count", float64(snapshot.TotalMembers - snapshot.Count), UnitCount},
		{"positive_count", float64(snapshot.PositiveCount), UnitCount},
		{"negative_count", float64(snapshot.NegativeCount), UnitCount},
		{"zero_count", float64(snapshot.ZeroCount), UnitCount},
		{"signed_median", snapshot.SignedMedian, UnitDimensionless},
		{"mean_absolute", snapshot.MeanAbsolute, UnitDimensionless},
		{"median_absolute", snapshot.MedianAbsolute, UnitDimensionless},
		{"mad", snapshot.Mad, UnitDimensionless},
		{"magnitude_mad", snapshot.MagnitudeMad, UnitDimensionless},
		{"interquartile_range", snapshot.Iqr, UnitDimensionless},
		{"rms", snapshot.Rms, UnitDimensionless},
		{"extreme_magnitude", snapshot.ExtremeMagnitude, UnitDimensionless},
		{"extreme_signed", snapshot.ExtremeSigned, UnitDimensionless},
		{"extreme_tie_count", float64(snapshot.ExtremeTieCount), UnitCount},
		{"peer_median_absolute", snapshot.PeerMedianAbsolute, UnitDimensionless},
		{"peer_mad", snapshot.PeerMad, UnitDimensionless},
		{"max_age", snapshot.MaxAge, UnitSecond},
		{"mean_age", snapshot.MeanAge, UnitSecond},
		{"median_age", snapshot.MedianAge, UnitSecond},
		{"median_from_age", snapshot.MedianFromAge, UnitSecond},
		{"focal_age", snapshot.FocalAge, UnitSecond},
		{"focal_from_age", snapshot.FocalFromAge, UnitSecond},
	}

	for _, fact := range facts {
		m.Metrics[fact.label] = m.Metrics[fact.label].Write(fact.value)
	}

	for name, view := range snapshot.Aggregates {
		m.Metrics[name] = m.Metrics[name].Write(view.Value)

		if !view.Ready {
			continue
		}

		m.Metrics[name+"_baseline"] = m.Metrics[name+"_baseline"].Write(view.Baseline)
		m.Metrics[name+"_divergence"] = m.Metrics[name+"_divergence"].Write(view.Divergence)
		m.Metrics[name+"_zscore"] = m.Metrics[name+"_zscore"].Write(view.ZScore)
		m.Metrics[name+"_velocity"] = m.Metrics[name+"_velocity"].Write(view.Velocity)
	}

	m.Metadata[MetadataSupport] = float64(snapshot.Count)

	if breadth, ready := snapshot.Aggregates["signed_fraction"]; ready {
		m.Metadata[MetadataDivergence] = breadth.Divergence

		if breadth.NoiseVariance > 0 {
			m.Metadata[MetadataNoiseVariance] = breadth.NoiseVariance
		}
	}

	if snapshot.ExtremeTieCount == 0 {
		m.Provenance = map[string]string{"extreme_key": snapshot.ExtremeKey}
	}
}

func (op *CrossSectionFacts) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = fmt.Errorf("%w: %s", err, op.label)
		}
	}

	return op.err
}
