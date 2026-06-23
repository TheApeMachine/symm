package fluid

import (
	"math"

	"github.com/theapemachine/nomagique/statistic"
)

func fluidflowFeatureBatch(reading fluidReading, changePct, volume float64) []float64 {
	if reading.price <= 0 || reading.spreadBPS <= 0 || volume <= 0 {
		return nil
	}

	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return nil
	}

	if reading.viscosity <= 0 {
		return nil
	}

	reynoldsHistory := reading.dynamics.reynoldsHistory
	divergenceHistory := reading.dynamics.divergenceHistory

	laminarCeiling := 0.0
	turbulentFloor := 0.0
	turbulentReady := 0.0
	divergenceEdge := 0.0

	if len(reynoldsHistory) >= minFluidDynamicsHistory {
		laminarCeiling, _ = statistic.MedianOf(reynoldsHistory)
		turbulentFloor, _ = statistic.QuantileOf(0.75, reynoldsHistory)
		turbulentReady = 1
	}

	if len(divergenceHistory) >= minFluidDynamicsHistory {
		divergenceEdge, _ = statistic.MedianOf(divergenceHistory)
	}

	if laminarCeiling <= 0 && reading.reynolds > 0 && !math.IsInf(reading.reynolds, 0) {
		laminarCeiling = reading.reynolds * (1 + reading.spreadBPS/10000)
	}

	if divergenceEdge <= 0 && reading.viscosity > 0 {
		divergenceEdge = math.Max(math.Abs(reading.divergence), reading.viscosity)
	}

	icebergScore := reading.dynamics.icebergScore(reading.midAddRate, reading.midExecuteRate)

	return []float64{
		reading.reynolds,
		math.Abs(reading.divergence),
		reading.viscosity,
		reading.midAddRate,
		reading.midExecuteRate,
		laminarCeiling,
		turbulentFloor,
		turbulentReady,
		divergenceEdge,
		icebergScore,
		reading.vorticity,
		reading.turbulence,
		reading.memory,
		reading.price,
		reading.spreadBPS,
		changePct,
		volume,
	}
}
