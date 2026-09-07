package equation

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/* PriorMoments owns normalized reliability-weighted moments and their causal clock. */
type PriorMoments struct {
	Samples, Pending, LastEpoch   uint64
	Mean, Weight, Support, Moment float64
}

/* PriorSummary separates completion, support, dispersion and retained authority. */
type PriorSummary struct {
	Samples, Pending                     uint64
	Defined, VarianceDefined             bool
	Mean, Variance, Support, Maturity    float64
	EvidenceAuthority, Authority, Memory float64
}

/* Age discounts total weight; normalized moment and support are scale invariant. */
func (moments *PriorMoments) Age(epoch uint64, memory float64) {
	if epoch <= moments.LastEpoch {
		return
	}

	if memory > 1 {
		gap := float64(epoch - moments.LastEpoch)
		moments.Weight *= math.Exp(gap * math.Log(1-1/memory))
	}
	moments.LastEpoch = epoch
}

/* Observe records zero-authority completions without aging or inventing evidence. */
func (moments *PriorMoments) Observe(value, authority, memory float64, epoch ...uint64) error {
	if !(authority >= 0 && authority <= 1) {
		return fmt.Errorf("%w: prior authority must be in [0, 1]", core.ErrDomain)
	}
	moments.Samples++

	if authority == 0 {
		return nil
	}

	if len(epoch) > 0 {
		moments.Age(epoch[0], memory)
	}

	if len(epoch) == 0 && memory > 1 {
		moments.Weight *= math.Exp(math.Log(1 - 1/memory))
	}

	if moments.Weight == 0 {
		moments.Mean, moments.Weight = value, authority
		moments.Support, moments.Moment = 1, 0
		return nil
	}
	total := moments.Weight + authority
	retained, incoming := moments.Weight/total, authority/total
	difference := value - moments.Mean
	moments.Support = 1 / (retained*retained/moments.Support + incoming*incoming)
	moments.Moment = retained*moments.Moment + (retained*incoming)*(difference*difference)
	moments.Mean += incoming * difference
	moments.Weight = total
	return nil
}

/* Summary computes the same reliability-weighted variance and signal authority. */
func (moments PriorMoments) Summary(memory float64) PriorSummary {
	reading := PriorSummary{Samples: moments.Samples, Pending: moments.Pending, Memory: memory}

	if moments.Weight <= 0 {
		return reading
	}
	reading.Defined, reading.Mean, reading.Support = true, moments.Mean, moments.Support
	reading.EvidenceAuthority = moments.Weight / moments.Support

	if moments.Support <= 1 {
		return reading
	}
	reading.VarianceDefined = true
	reading.Variance = moments.Moment * (moments.Support / (moments.Support - 1))
	reading.Maturity = (moments.Support - 1) / moments.Support
	power := moments.Mean * moments.Mean
	totalPower := power + reading.Variance

	if totalPower > 0 {
		reading.Authority = (reading.Maturity * reading.EvidenceAuthority) * (power / totalPower)
	}
	return reading
}
