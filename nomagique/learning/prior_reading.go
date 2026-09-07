package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
PriorReading projects the canonical PriorMoments summary. Pending and context
depth belong to Model's issue/resolve lifecycle; projection does not estimate.
*/
type PriorReading struct {
	Depth             int
	ContextLength     int
	Pending           uint64
	Samples           uint64
	Defined           bool
	Mean              float64
	Variance          float64
	VarianceDefined   bool
	Support           float64
	Maturity          float64
	EvidenceAuthority float64
	Authority         float64
	Memory            float64
}

/* ProjectPrior decodes the graph's output without recomputing its estimator facts. */
func ProjectPrior(fields map[string]core.Primitive) (PriorReading, error) {
	decoder := core.NewDecoder(fields)
	reading := PriorReading{
		Samples:           core.Decode[uint64](decoder, "samples"),
		Defined:           core.Decode[bool](decoder, "defined"),
		Mean:              core.Decode[float64](decoder, "mean"),
		Variance:          core.Decode[float64](decoder, "variance"),
		VarianceDefined:   core.Decode[bool](decoder, "variance_defined"),
		Support:           core.Decode[float64](decoder, "support"),
		Maturity:          core.Decode[float64](decoder, "maturity"),
		EvidenceAuthority: core.Decode[float64](decoder, "evidence_authority"),
		Authority:         core.Decode[float64](decoder, "authority"),
		Memory:            core.Decode[float64](decoder, "memory"),
	}
	return reading, decoder.Error()
}

/* SamplingVariance evaluates the canonical specificity-debt equation. */
func (reading PriorReading) SamplingVariance() float64 {
	variance, err := equation.SamplingVariance(
		float64(reading.Depth), float64(reading.ContextLength), reading.Support, reading.Variance,
	)

	if err != nil {
		panic(err)
	}
	return variance
}
