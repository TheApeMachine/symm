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
the round-trip friction (fees plus half-spread when spreadBPS is known).
*/
func entryClearsFriction(score float64, feePct float64, spreadBPS float64) bool {
	slippageBPS := config.System.SlippageBPS

	if spreadBPS > 0 {
		slippageBPS = spreadBPS
	}

	friction := roundTripFrictionPct(feePct, slippageBPS)
	required := config.System.EntryEdgeMultiple * friction

	if required <= 0 {
		return score > 0
	}

	requiredSNR := required * 100

	return score >= requiredSNR
}
