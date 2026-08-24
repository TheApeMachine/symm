package data

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Binding maps one pipeline output slot to a named metric with its unit and
timescale. It is the declarative link between a computed value in a Frame and
the Metric it projects into.
*/
type Binding struct {
	From      types.Symbol
	Name      string
	Unit      Unit
	Timescale Timescale
}

/*
Projector projects one evaluated Frame into one finalized Measurement. It is
the output step of a signal: the pipeline computes values, the projector names
them, and the measurement derives its own Maturity and SNR from estimator
metadata the pipeline carries. A signal declares its bindings once in its
constructor and applies them per observation.
*/
type Projector struct {
	bindings []Binding
}

func NewProjector(bindings ...Binding) *Projector {
	return &Projector{bindings: append([]Binding(nil), bindings...)}
}

/*
Project maps every populated binding slot into the Measurement and finalizes
quality from the frame's estimator metadata. `label`, `source`, `at`, and
`from` are provenance injected by the caller (the signal's Step holds them),
not stored in the projector.
*/
func (projector *Projector) Project(
	label string,
	source string,
	at time.Time,
	from time.Time,
	frame types.Frame,
) *Measurement[float64] {
	id := fmt.Sprintf("%s:%s:%d", source, label, at.UnixNano())
	measurement := NewMeasurement[float64](id, label, source, at, from)
	measurement.Metadata = map[string]float64{}

	if frame.Err != nil {
		measurement.Err = frame.Err

		return measurement
	}

	for _, binding := range projector.bindings {
		value, found := frame.Get(binding.From)

		if !found {
			continue
		}

		measurement.PutMetric(NewMetric(
			binding.Name, value, nil, nil, binding.Unit, binding.Timescale,
		))
	}

	// Carry the estimator facts into metadata so Finalize derives quality.
	if support, found := frame.Get(types.SampleCount); found {
		measurement.Metadata[MetadataSupport] = support
	}

	if divergence, found := frame.Get(types.MustIntern("divergence")); found {
		measurement.Metadata[MetadataDivergence] = divergence
	}

	if noise, found := frame.Get(types.MustIntern("noise_variance")); found {
		measurement.Metadata[MetadataNoiseVariance] = noise
	}

	if mahalanobis, found := frame.Get(types.MustIntern("mahalanobis/snr")); found {
		measurement.Metadata[MetadataMahalanobisSNR] = mahalanobis
	}

	measurement.Finalize()

	return measurement
}
