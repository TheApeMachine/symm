package prior

import (
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
Reading projects the canonical PriorMoments summary. Pending and context
depth come from the cognition query and agent lifecycle; projection does not estimate.
*/
type Reading struct {
	Provisional       bool
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

func fromSummary(summary equation.PriorSummary) Reading {
	return Reading{
		Pending:           summary.Pending,
		Samples:           summary.Samples,
		Defined:           summary.Defined,
		Mean:              summary.Mean,
		Variance:          summary.Variance,
		VarianceDefined:   summary.VarianceDefined,
		Support:           summary.Support,
		Maturity:          summary.Maturity,
		EvidenceAuthority: summary.EvidenceAuthority,
		Authority:         summary.Authority,
		Memory:            summary.Memory,
	}
}

/* SamplingVariance evaluates the canonical specificity-debt equation. */
func (reading Reading) SamplingVariance() float64 {
	variance, err := equation.SamplingVariance(
		float64(reading.Depth), float64(reading.ContextLength), reading.Support, reading.Variance,
	)

	if err != nil {
		panic(err)
	}

	return variance
}
