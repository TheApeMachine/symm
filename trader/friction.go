package trader

import (
	"github.com/theapemachine/symm/config"
)

/*
roundTripFrictionPct is the estimated round-trip cost (fees + slippage) as a
fraction of notional. At SNR 1 the playbook assumes one sigma of edge equals this
friction; EntryEdgeMultiple scales the required thesis score.
*/
func roundTripFrictionPct(feePct, slippageBps float64) float64 {
	return 2*feePct/100 + slippageBps/10000
}

/*
entryClearsFriction requires the thesis score to clear EntryEdgeMultiple times
the friction floor expressed in SNR units (score is RMS of playbook SNRs).
*/
func entryClearsFriction(score float64, feePct float64) bool {
	friction := roundTripFrictionPct(feePct, config.System.SlippageBPS)
	required := config.System.EntryEdgeMultiple * friction

	if required <= 0 {
		return score > 0
	}

	// Map friction pct into the same scale as typical playbook scores (~1–4 SNR).
	requiredSNR := required * 100

	return score >= requiredSNR
}
