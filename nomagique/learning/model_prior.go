package learning

import (
	"github.com/theapemachine/symm/nomagique/equation"
)

/* modelPrior owns a context's fixed numeric moments and unresolved tickets. */
type modelPrior struct {
	state   equation.PriorMoments
	memory  float64
	pending uint64
}

func (prior *modelPrior) reading(epoch uint64) PriorReading {
	prior.state.Age(epoch, prior.memory)
	summary := prior.state.Summary(prior.memory)
	reading := PriorReading{
		Samples: summary.Samples, Defined: summary.Defined,
		Mean: summary.Mean, Variance: summary.Variance, VarianceDefined: summary.VarianceDefined,
		Support: summary.Support, Maturity: summary.Maturity,
		EvidenceAuthority: summary.EvidenceAuthority, Authority: summary.Authority, Memory: summary.Memory,
	}
	reading.Pending = prior.pending
	return reading
}
