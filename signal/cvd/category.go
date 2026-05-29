package cvd

import (
	"math"

	"github.com/theapemachine/symm/engine"
)

/*
cvdCategory maps executed-flow shape onto the absorption perspective.
*/
func cvdCategory(
	netFraction float64,
	drift float64,
	tradeCount int,
) engine.Category {
	if tradeCount < cvdMinTrades {
		return engine.CatVolumeStarvation
	}

	directional := (math.Abs(netFraction) - cvdMinNetFraction) / (1 - cvdMinNetFraction)

	if directional <= 0 {
		return engine.CatStochasticBalance
	}

	if netFraction > 0 && drift <= cvdPriceFlatBand {
		return engine.CatHiddenAbsorption
	}

	if netFraction < 0 && drift >= -cvdPriceFlatBand {
		return engine.CatHiddenAbsorption
	}

	return engine.CatAggressiveDrive
}
