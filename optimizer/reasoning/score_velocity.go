package reasoning

import (
	"github.com/theapemachine/symm/optimizer/replay"
)

/*
walletVelocityScore maximises EUR gained per unit of capital-at-risk time: realized
wallet growth scaled by how quickly positions turn over (fewer exposure ticks for the
same EUR beats sitting in one trade).
*/
func walletVelocityScore(result replay.ReplayResult) float64 {
	if result.RealizedEUR <= 0 {
		return result.RealizedEUR
	}

	credited := result.RealizedEUR

	if result.ExposureTicks <= 0 {
		return credited
	}

	timeEfficiency := float64(result.TotalTicks) / float64(result.ExposureTicks)

	return credited * (1 + velocityWeight*(timeEfficiency-1))
}
