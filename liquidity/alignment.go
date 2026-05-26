package liquidity

import "github.com/theapemachine/symm/engine"

/*
liquidityConfidence scores how illiquid the current symbol is versus peers.
With peer context it combines illiquidity depth and cross-section lead; alone it
uses the illiquidity score directly so a single reading is not pinned at 50%.
*/
func liquidityConfidence(score float64, peers []float64) float64 {
	if score <= 0 {
		return 0
	}

	if len(peers) > 0 {
		maxPeer := 0.0

		for _, peer := range peers {
			if peer > maxPeer {
				maxPeer = peer
			}
		}

		if score > maxPeer {
			margin := (score - maxPeer) / score

			return engine.AlignConfidence(score, margin)
		}
	}

	return engine.ConfidenceFromScore(score)
}
