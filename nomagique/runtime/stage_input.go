package runtime

import (
	"time"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
StageInput is the read-only prior evidence for one workspace sequence.

Handlers in one concurrent HandlerGroup receive the SAME StageInput. A sibling
MUST NOT become visible merely because it happened to finish first: StageInput
contains only completed earlier-stage outputs for the same sequence.

StageInput is owned by the workspace runtime. Producers use its accessors to
read ingress identity and prior-stage metrics without copying them into their
own Metrics namespace.
*/
type StageInput struct {
	seq     int64
	ingress *data.Measurement[float64]

	// priorOutputs holds the completed WORM outputs of stages before the
	// consumer's stage, indexed [stageIdx][nodeIdx]. Each pointer is a
	// published, immutable producer output from a ProducerRing slot.
	priorOutputs [][]*data.Measurement[float64]
}

// NewStageInput constructs a read-only view for the given sequence.
func NewStageInput(
	seq int64,
	ingress *data.Measurement[float64],
	priorOutputs [][]*data.Measurement[float64],
) *StageInput {
	return &StageInput{
		seq:          seq,
		ingress:      ingress,
		priorOutputs: priorOutputs,
	}
}

// Seq returns the workspace sequence number.
func (si *StageInput) Seq() int64 { return si.seq }

// Ingress returns the original ingress measurement (the websocket event).
func (si *StageInput) Ingress() *data.Measurement[float64] { return si.ingress }

// Symbol returns the ingress label (symbol name).
func (si *StageInput) Symbol() string {
	if si.ingress == nil {
		return ""
	}

	return si.ingress.Label
}

// At returns the ingress event time.
func (si *StageInput) At() time.Time {
	if si.ingress == nil {
		return time.Time{}
	}

	return si.ingress.At
}

// From returns the ingress window start time.
func (si *StageInput) From() time.Time {
	if si.ingress == nil {
		return time.Time{}
	}

	return si.ingress.From
}

// IngressMetric reads a metric from the original ingress measurement.
func (si *StageInput) IngressMetric(key string) (data.Metric[float64], bool) {
	if si.ingress == nil {
		return data.Metric[float64]{}, false
	}

	return si.ingress.LookupMetric(key)
}

// IngressProvenance reads a provenance value from the original ingress.
func (si *StageInput) IngressProvenance(key string) (string, bool) {
	if si.ingress == nil {
		return "", false
	}

	return si.ingress.GetProvenance(key)
}

// IngressMetadata reads a metadata value from the original ingress.
func (si *StageInput) IngressMetadata(key string) (string, bool) {
	if si.ingress == nil {
		return "", false
	}

	return si.ingress.GetMetadata(key)
}

/*
ProducerOutput returns a completed prior-stage producer's WORM output by
source name. It searches completed earlier stages only — siblings in the same
concurrent HandlerGroup are invisible.
*/
func (si *StageInput) ProducerOutput(source string) *data.Measurement[float64] {
	for _, stageOutputs := range si.priorOutputs {
		for _, output := range stageOutputs {
			if output != nil && output.Source == source {
				return output
			}
		}
	}

	return nil
}

/*
ProducerMetric reads a specific metric from a prior-stage producer's output.
*/
func (si *StageInput) ProducerMetric(source, key string) (data.Metric[float64], bool) {
	output := si.ProducerOutput(source)
	if output == nil {
		return data.Metric[float64]{}, false
	}

	return output.LookupMetric(key)
}

/*
AllPriorOutputs returns all completed prior-stage outputs as a flat slice.
Useful for stages like Category that need to iterate all prior signal results.
The returned slice is a shallow copy safe for iteration; the pointed-to
Measurements are immutable WORM objects.
*/
func (si *StageInput) AllPriorOutputs() []*data.Measurement[float64] {
	var count int
	for _, stageOutputs := range si.priorOutputs {
		count += len(stageOutputs)
	}

	if count == 0 {
		return nil
	}

	result := make([]*data.Measurement[float64], 0, count)
	for _, stageOutputs := range si.priorOutputs {
		for _, output := range stageOutputs {
			if output != nil {
				result = append(result, output)
			}
		}
	}

	return result
}

/*
StageOutputs returns the completed outputs from one specific prior stage.
stageIdx is relative to the consumer's stage (0 = first stage).
Returns nil if the stage index is out of range.
*/
func (si *StageInput) StageOutputs(stageIdx int) []*data.Measurement[float64] {
	if stageIdx < 0 || stageIdx >= len(si.priorOutputs) {
		return nil
	}

	return si.priorOutputs[stageIdx]
}
