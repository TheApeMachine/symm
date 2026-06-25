package manifold

import (
	"math"
	"sort"

	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

func returnAnalyticPhase(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}

	tail := returns

	if len(returns) > 2 {
		window := int(math.Ceil(math.Sqrt(float64(len(returns)))))

		if window < 2 {
			window = 2
		}

		if window < len(returns) {
			tail = returns[len(returns)-window:]
		}
	}

	realPart := 0.0
	imagPart := 0.0

	for index, value := range tail {
		weight := float64(index + 1)
		realPart += value * weight

		if index > 0 {
			imagPart += (value - tail[index-1]) * weight
		}
	}

	if realPart == 0 && imagPart == 0 {
		return 0
	}

	angle := math.Atan2(imagPart, realPart)

	if angle < 0 {
		angle += 2 * math.Pi
	}

	return angle
}

func oscillatorForSolver(
	oscillator mkernel.Oscillator,
	config mkernel.Config,
) mkernel.Oscillator {
	solverOscillator := oscillator
	posX, posY, posZ := solverCarrierPosition(config)

	if solverOscillator.PosX == 0 && solverOscillator.PosY == 0 && solverOscillator.PosZ == 0 {
		solverOscillator.PosX = posX
		solverOscillator.PosY = posY
		solverOscillator.PosZ = posZ
	}

	return solverOscillator
}

func solverCarrierPosition(config mkernel.Config) (posX, posY, posZ float64) {
	gridX := float64(max(config.GridX, 1))
	gridY := float64(max(config.GridY, 1))
	gridZ := float64(max(config.GridZ, 1))

	return 1 / gridX, 1 / gridY, 1 / gridZ
}

func normalizeOscillatorsForSolver(
	oscillators []mkernel.Oscillator,
	rhoMin float64,
	maxModes uint32,
) []mkernel.Oscillator {
	if len(oscillators) == 0 {
		return oscillators
	}

	normalizationCount := float64(maxModes)

	if normalizationCount <= 0 {
		normalizationCount = float64(len(oscillators))
	}

	normalized := make([]mkernel.Oscillator, len(oscillators))

	for index, oscillator := range oscillators {
		normalized[index] = oscillator
		perCarrierEnergy := math.Max(oscillator.Heat, rhoMin) / normalizationCount
		normalized[index].Amplitude = math.Sqrt(perCarrierEnergy)
		normalized[index].Heat = perCarrierEnergy
	}

	return normalized
}

func returnFrequency(returns []float64, deltaT float64) float64 {
	if deltaT <= 0 {
		return 0
	}

	if len(returns) < 2 {
		return 2 * math.Pi / deltaT
	}

	mean := 0.0

	for _, value := range returns {
		mean += value
	}

	mean /= float64(len(returns))

	variance := 0.0

	for _, value := range returns {
		delta := value - mean
		variance += delta * delta
	}

	variance /= float64(len(returns) - 1)

	if variance <= 0 {
		return 2 * math.Pi / deltaT
	}

	return math.Sqrt(variance) / deltaT
}

func tradeSideSign(side string) float64 {
	if side == "sell" {
		return -1
	}

	return 1
}

func truncateLevels(levels []BookLevel, depth int) []BookLevel {
	if depth <= 0 || len(levels) <= depth {
		return levels
	}

	return levels[:depth]
}

func capSolverCarriers(
	symbolOscillators []mkernel.Oscillator,
	symbolCarriers []fieldCarrier,
	whaleOscillators []mkernel.Oscillator,
	whaleCarriers []fieldCarrier,
	maxModes uint32,
) ([]mkernel.Oscillator, []fieldCarrier) {
	limit := int(maxModes)
	solverOscillators, solverCarriers := capCarriers(
		symbolOscillators,
		symbolCarriers,
		uint32(limit),
	)

	if len(solverOscillators) >= limit {
		return solverOscillators, solverCarriers
	}

	remaining := uint32(limit - len(solverOscillators))
	trimmedWhaleOscillators, trimmedWhaleCarriers := capCarriers(
		whaleOscillators,
		whaleCarriers,
		remaining,
	)

	solverOscillators = append(solverOscillators, trimmedWhaleOscillators...)
	solverCarriers = append(solverCarriers, trimmedWhaleCarriers...)

	return solverOscillators, solverCarriers
}

func capCarriers(
	oscillators []mkernel.Oscillator,
	carriers []fieldCarrier,
	maxCount uint32,
) ([]mkernel.Oscillator, []fieldCarrier) {
	limit := int(maxCount)

	if limit <= 0 || len(oscillators) <= limit {
		return oscillators, carriers
	}

	indices := make([]int, len(oscillators))

	for index := range oscillators {
		indices[index] = index
	}

	sort.Slice(indices, func(leftIndex, rightIndex int) bool {
		leftHeat := oscillators[indices[leftIndex]].Heat
		rightHeat := oscillators[indices[rightIndex]].Heat

		return leftHeat > rightHeat
	})

	trimmedOscillators := make([]mkernel.Oscillator, limit)
	trimmedCarriers := make([]fieldCarrier, limit)

	for rank := 0; rank < limit; rank++ {
		sourceIndex := indices[rank]
		trimmedOscillators[rank] = oscillators[sourceIndex]
		trimmedCarriers[rank] = carriers[sourceIndex]
	}

	return trimmedOscillators, trimmedCarriers
}
