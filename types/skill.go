package types

import "math"

/*
Skill owns forecast-epoch weight for trail distance scaling. Stoploss embeds
it so Weight stays on the public stop surface.
*/
type Skill struct {
	Weight            float64 `json:"weight"`
	lastConsumedEpoch uint64
}

/*
NewSkill returns a unit-weight skill shell ready for Seed or Reweight.
*/
func NewSkill() Skill {
	return Skill{Weight: 1}
}

/*
Seed sets Weight from the signal share of expected-return magnitude when σ is
present (|ER| / (|ER|+σ)), else cognition confidence. High weight means an
informative forecast — the same direction Reweight uses for residual skill.
*/
func (skill *Skill) Seed(evidence StopEvidence) {
	if evidence.Uncertainty > 0 {
		magnitude := math.Abs(evidence.ExpectedReturn) + evidence.Uncertainty
		share := math.Abs(evidence.ExpectedReturn) / magnitude

		if share <= 0 {
			skill.Weight = 1
			return
		}

		skill.Weight = share
		return
	}

	if evidence.CognitionReady &&
		evidence.CognitionConfidence > 0 &&
		evidence.CognitionConfidence <= 1 {
		skill.Weight = evidence.CognitionConfidence
		return
	}

	skill.Weight = 1
}

/*
Reweight moves Weight toward residual-relative skill once per resolved
forecast epoch. Upside updates are damped; downside updates cut faster.
*/
func (skill *Skill) Reweight(evidence StopEvidence) {
	if !evidence.ReturnReady || evidence.ForecastEpoch == skill.lastConsumedEpoch {
		return
	}

	skill.lastConsumedEpoch = evidence.ForecastEpoch
	residualSkill := 1.0

	if evidence.NormalizedResidual > 0 {
		residualSkill = 1 / (1 + evidence.NormalizedResidual)
	}

	delta := residualSkill - skill.Weight

	if delta >= 0 {
		skill.Weight += delta * residualSkill * residualSkill
	}

	if delta < 0 {
		skill.Weight += delta * (1 - residualSkill)
	}

	skill.Weight = math.Min(1, math.Max(math.Nextafter(0, 1), skill.Weight))
}
