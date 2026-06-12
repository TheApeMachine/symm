package fluid

import "math"

/*
rusanovFlux1D is the local Lax-Friedrichs face flux for the 1D continuity equation.
*/
func rusanovFlux1D(
	fluxLeft, fluxRight, rhoLeft, rhoRight, waveSpeed float64,
) float64 {
	return 0.5*(fluxLeft+fluxRight) - 0.5*waveSpeed*(rhoRight-rhoLeft)
}

func (grid *FluidGrid) computeRHS(
	rho []float64,
	rhs []float64,
	sources []float64,
) {
	cellCount := len(rho)
	invDx := 1.0 / grid.tickSize
	invDxSq := invDx * invDx

	for index := 1; index < cellCount-1; index++ {
		rhoLeft := rho[index-1]
		rhoCenter := rho[index]
		rhoRight := rho[index+1]

		velocityLeft := grid.velocity[index-1]
		velocityCenter := grid.velocity[index]
		velocityRight := grid.velocity[index+1]

		fluxLeft := rhoLeft * velocityLeft
		fluxCenter := rhoCenter * velocityCenter
		fluxRight := rhoRight * velocityRight

		waveLeft := math.Max(math.Abs(velocityLeft), math.Abs(velocityCenter))
		waveRight := math.Max(math.Abs(velocityCenter), math.Abs(velocityRight))

		faceLeft := rusanovFlux1D(fluxLeft, fluxCenter, rhoLeft, rhoCenter, waveLeft)
		faceRight := rusanovFlux1D(fluxCenter, fluxRight, rhoCenter, rhoRight, waveRight)

		advection := -(faceRight - faceLeft) * invDx

		diffusion := 0.0

		if grid.diffusionCoeff > 0 {
			diffusion = grid.diffusionCoeff *
				(rhoRight - 2*rhoCenter + rhoLeft) * invDxSq
		}

		rhs[index] = advection + diffusion + sources[index]
	}
}

func (grid *FluidGrid) applyNeumannBoundary(rho []float64) {
	cellCount := len(rho)

	if cellCount < 2 {
		return
	}

	rho[0] = rho[1]
	rho[cellCount-1] = rho[cellCount-2]
}

func (grid *FluidGrid) integrateRK2(dt float64) {
	grid.computeRHS(grid.rho, grid.rhoK1, grid.sources)

	cellCount := len(grid.rho)

	for index := 1; index < cellCount-1; index++ {
		grid.rhoStage[index] = grid.rho[index] + dt*grid.rhoK1[index]
	}

	grid.applyNeumannBoundary(grid.rhoStage)

	grid.computeRHS(grid.rhoStage, grid.rhoK2, grid.sources)

	for index := 1; index < cellCount-1; index++ {
		grid.rho[index] += 0.5 * dt * (grid.rhoK1[index] + grid.rhoK2[index])
	}

	grid.applyNeumannBoundary(grid.rho)
}
