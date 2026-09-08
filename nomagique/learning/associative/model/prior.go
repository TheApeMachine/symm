package model

import (
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
)

/* modelPrior owns a context's fixed numeric moments and unresolved tickets. */
type modelPrior struct {
	State       equation.PriorMoments
	Provisional equation.PriorMoments
	Memory      float64
	pending     uint64
}

func (record *modelPrior) reading(epoch uint64) prior.Reading {
	record.State.Age(epoch, record.Memory)
	record.Provisional.Age(epoch, record.Memory)
	summary := record.State.Summary(record.Memory)
	provisional := !summary.Defined && record.Provisional.Samples > 0

	if provisional {
		summary = record.Provisional.Summary(record.Memory)
	}
	reading := prior.Reading{
		Samples: summary.Samples, Defined: summary.Defined,
		Mean: summary.Mean, Variance: summary.Variance, VarianceDefined: summary.VarianceDefined,
		Support: summary.Support, Maturity: summary.Maturity,
		EvidenceAuthority: summary.EvidenceAuthority, Authority: summary.Authority, Memory: summary.Memory,
	}
	reading.Pending = record.pending
	reading.Provisional = provisional
	return reading
}
