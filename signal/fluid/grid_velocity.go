package fluid

import "math"

/*
inferVelocityField recovers the spatially varying quote-drift velocity from
inter-cell mass flux and mid-price advection.
*/
func (grid *FluidGrid) inferVelocityField(currentMid, dt float64) {
	cellCount := len(grid.velocity)

	grid.midPriceVelocity = (currentMid - grid.prevMidPrice) / dt

	if cellCount < 3 {
		return
	}

	sourceMagnitude := 0.0

	for _, volume := range grid.sourceAccumulator {
		sourceMagnitude += math.Abs(volume)
	}

	if sourceMagnitude == 0 && math.Abs(grid.midPriceVelocity) < rhoFloor {
		grid.clearField(grid.velocity)

		return
	}

	invDt := 1.0 / dt

	for index := 1; index < cellCount-1; index++ {
		rhoFaceLeft := 0.5 * (grid.observedRho[index] + grid.remappedRho[index])
		rhoFaceRight := 0.5 * (grid.observedRho[index+1] + grid.remappedRho[index+1])

		velocityLeft := grid.midPriceVelocity
		velocityRight := grid.midPriceVelocity

		massFluxLeft := (grid.observedRho[index] - grid.remappedRho[index]) * invDt

		if rhoFaceLeft > rhoFloor {
			velocityLeft = massFluxLeft / rhoFaceLeft
		}

		massFluxRight := (grid.observedRho[index+1] - grid.remappedRho[index+1]) * invDt

		if rhoFaceRight > rhoFloor {
			velocityRight = massFluxRight / rhoFaceRight
		}

		remapResidual := (grid.observedRho[index] - grid.remappedRho[index]) * invDt
		gradRho := (grid.observedRho[index+1] - grid.observedRho[index-1]) / (2 * grid.tickSize)

		velocityCorrection := 0.0

		gradFloor := math.Max(rhoFloor, grid.rhoGradFloor(index))

		if math.Abs(gradRho) > gradFloor {
			sourceRate := grid.sourceAccumulator[index] * invDt
			velocityCorrection = -(remapResidual - sourceRate) / gradRho
		}

		grid.velocity[index] = 0.5*(velocityLeft+velocityRight) + velocityCorrection
	}

	grid.velocity[0] = grid.velocity[1]
	grid.velocity[cellCount-1] = grid.velocity[cellCount-2]
}

func (grid *FluidGrid) estimateDiffusionCoefficient() float64 {
	cellCount := len(grid.velocity)

	if cellCount < 3 {
		return 0
	}

	shearSum := 0.0
	shearCount := 0

	for index := 1; index < cellCount-1; index++ {
		shear := math.Abs(grid.velocity[index+1]-grid.velocity[index-1]) / (2 * grid.tickSize)
		shearSum += shear * grid.tickSize
		shearCount++
	}

	if shearCount == 0 {
		return 0
	}

	return shearSum / float64(shearCount)
}
