package fluid

import "math"

/*
inferVelocityField recovers the spatially varying quote-drift velocity from
inter-cell mass flux and mid-price advection.
*/
func (grid *FluidGrid) inferVelocityField(currentMid, dt float64) {
	cellCount := len(grid.velocity)

	grid.midPriceVelocity = grid.resolveVelocity(
		(currentMid-grid.prevMidPrice)/dt,
		dt,
	)

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

		massFluxLeft := (grid.observedRho[index] - grid.remappedRho[index]) *
			grid.tickSize * invDt

		if rhoFaceLeft > rhoFloor {
			velocityLeft = massFluxLeft / rhoFaceLeft
		}

		massFluxRight := (grid.observedRho[index+1] - grid.remappedRho[index+1]) *
			grid.tickSize * invDt

		if rhoFaceRight > rhoFloor {
			velocityRight = massFluxRight / rhoFaceRight
		}

		remapResidual := (grid.observedRho[index] - grid.remappedRho[index]) * invDt
		gradRho := (grid.observedRho[index+1] - grid.observedRho[index-1]) / (2 * grid.tickSize)

		velocityCorrection := 0.0

		gradFloor := grid.rhoGradFloor(index) / grid.spatialSpan()

		if math.Abs(gradRho) > gradFloor {
			sourceRate := grid.sourceAccumulator[index] * invDt
			velocityCorrection = -(remapResidual - sourceRate) / gradRho
		}

		grid.velocity[index] = grid.resolveVelocity(
			0.5*(velocityLeft+velocityRight)+velocityCorrection,
			dt,
		)
	}

	grid.velocity[0] = grid.velocity[1]
	grid.velocity[cellCount-1] = grid.velocity[cellCount-2]
}

/*
resolveVelocity projects an inferred velocity onto the spatial range resolved by
one lattice observation. Motion beyond the full grid span during dt is aliased by
the finite book window and cannot be distinguished from boundary exit.
*/
func (grid *FluidGrid) resolveVelocity(velocity, dt float64) float64 {
	limit := grid.spatialSpan() / dt

	return math.Max(-limit, math.Min(velocity, limit))
}

/*
spatialSpan returns the represented price span so transport limits remain tied
to grid resolution.
*/
func (grid *FluidGrid) spatialSpan() float64 {
	return float64(len(grid.velocity)-1) * grid.tickSize
}

/*
estimateDiffusionCoefficient derives diffusion from observed grid dynamics so
integration uses the market's current smoothing scale.
*/
func (grid *FluidGrid) estimateDiffusionCoefficient() float64 {
	cellCount := len(grid.velocity)

	if cellCount < 3 {
		return 0
	}

	shearSum := 0.0
	shearCount := 0

	for index := 1; index < cellCount-1; index++ {
		shear := math.Abs(grid.velocity[index+1]-grid.velocity[index-1]) / (2 * grid.tickSize)
		shearSum += shear * grid.tickSize * grid.tickSize
		shearCount++
	}

	if shearCount == 0 {
		return 0
	}

	return shearSum / float64(shearCount)
}
