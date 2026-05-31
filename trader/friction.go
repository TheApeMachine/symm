package trader

import (
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
entryClearsFriction requires thesis_score to clear RequiredThesisScore for this
symbol's fees and spread (all in playbook sigma units).
*/
func entryClearsFriction(
	score float64,
	entryEdgeMultiple float64,
	feePct float64,
	spreadBPS float64,
) bool {
	slippageBPS := config.System.SlippageBPS

	if spreadBPS > 0 {
		slippageBPS = spreadBPS
	}

	requiredSNR := perspectives.RequiredThesisScore(
		entryEdgeMultiple,
		feePct,
		slippageBPS,
	)

	if requiredSNR <= 0 {
		return score > 0
	}

	return score >= requiredSNR
}
