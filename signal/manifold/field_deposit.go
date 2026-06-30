package manifold

import (
	"fmt"
	"math"
)

func (field *Field) depositBook(state *UniverseState, activeCarriers int) error {
	if state.tickSize <= 0 {
		return fmt.Errorf("manifold: tick size must be positive for %q", state.symbol)
	}

	if activeCarriers <= 0 {
		return fmt.Errorf("manifold: active carrier count must be positive")
	}

	bids := truncateLevels(state.book.Bids, 1)
	asks := truncateLevels(state.book.Asks, 1)
	cellCount := int(field.config.GridX)

	if cellCount <= 0 {
		return fmt.Errorf("manifold: grid x must be positive")
	}

	bidDensity := make([]float64, cellCount)
	askDensity := make([]float64, cellCount)

	if depositErr := field.accumulateBookDensity(state, bids, activeCarriers, bidDensity); depositErr != nil {
		return depositErr
	}

	if depositErr := field.accumulateBookDensity(state, asks, activeCarriers, askDensity); depositErr != nil {
		return depositErr
	}

	filterManifoldDensity(bidDensity)
	filterManifoldDensity(askDensity)

	if depositErr := field.depositBookDensity(state, bidDensity, -1); depositErr != nil {
		return depositErr
	}

	if depositErr := field.depositBookDensity(state, askDensity, 1); depositErr != nil {
		return depositErr
	}

	return nil
}

func (field *Field) accumulateBookDensity(
	state *UniverseState,
	levels []BookLevel,
	activeCarriers int,
	density []float64,
) error {
	for _, level := range levels {
		if level.Qty <= 0 {
			continue
		}

		offsetTicks := (level.Price - state.midPrice) / state.tickSize
		coords := field.universe.coords(state, offsetTicks)

		if int(coords.cellX) >= len(density) {
			continue
		}

		rho, rhoErr := field.liquidityRho(state, level.Qty, activeCarriers)

		if rhoErr != nil {
			return rhoErr
		}

		density[coords.cellX] += rho
	}

	return nil
}

func (field *Field) depositBookDensity(
	state *UniverseState,
	density []float64,
	sideSign float64,
) error {
	coords := field.universe.coords(state, 0)

	for cellX, rho := range density {
		if rho <= 0 {
			continue
		}

		momentum := sideSign * rho

		if depositErr := field.solver.DepositCell(
			uint32(cellX),
			coords.cellY,
			coords.cellZ,
			rho,
			momentum,
			0,
			0,
			rho*field.config.CV,
		); depositErr != nil {
			return depositErr
		}
	}

	return nil
}

func filterManifoldDensity(density []float64) {
	if len(density) < 3 {
		return
	}

	total := finiteDensityMass(density)
	if total <= 0 {
		return
	}

	filtered := append([]float64(nil), density...)

	for index := 1; index < len(density)-1; index++ {
		left := finitePositive(density[index-1])
		center := finitePositive(density[index])
		right := finitePositive(density[index+1])
		localMass := left + center + right

		if localMass <= 0 {
			continue
		}

		curvature := math.Abs(left - 2*center + right)
		alpha := curvature / (curvature + localMass + math.SmallestNonzeroFloat64)
		target := localMass / 3
		delta := alpha * (target - center)

		filtered[index] += delta
		filtered[index-1] -= delta / 2
		filtered[index+1] -= delta / 2
	}

	filteredTotal := 0.0
	for index, value := range filtered {
		value = finitePositive(value)
		filtered[index] = value
		filteredTotal += value
	}

	if filteredTotal <= 0 {
		return
	}

	scale := total / filteredTotal
	for index := range density {
		density[index] = filtered[index] * scale
	}
}

func finiteDensityMass(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += finitePositive(value)
	}

	return total
}

func finitePositive(value float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value
}
