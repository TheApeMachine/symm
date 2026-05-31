package perspectives

import "math"

/*
AggregateThesisScore combines category SNR readings into one thesis_score (RMS in
sigma units). Only measurements in relevant categories contribute; SNR <= 0 is
skipped (warmup). No playbook-count multiplier — composition is additive via RMS.
*/
func AggregateThesisScore(
	measurements []Measurement,
	relevant map[CategoryType]bool,
) float64 {
	energy := 0.0
	observations := 0

	for _, measurement := range measurements {
		if len(relevant) > 0 && !relevant[measurement.Category] {
			continue
		}

		if measurement.SNR <= 0 {
			continue
		}

		energy += measurement.SNR * measurement.SNR
		observations++
	}

	if observations == 0 {
		return 0
	}

	return math.Sqrt(energy / float64(observations))
}

/*
RequiredThesisScore is the minimum thesis_score (sigma units) that must clear
round-trip friction scaled by EntryEdgeMultiple. Used for required_score and
score_cost_ratio in playbooks and desk gates.
*/
func RequiredThesisScore(entryEdgeMultiple, feePct, spreadBPS float64) float64 {
	friction := RoundTripFrictionPct(feePct, spreadBPS)

	if entryEdgeMultiple <= 0 || friction <= 0 {
		return 0
	}

	return entryEdgeMultiple * friction * 100
}

/*
RoundTripFrictionPct is the estimated round-trip cost (fees + full spread for
entry and exit) as a fraction of notional.
*/
func RoundTripFrictionPct(feePct, spreadBPS float64) float64 {
	return 2*feePct/100 + spreadBPS/10000
}
