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

/*
faceFluxPair returns the Rusanov face fluxes bracketing rho[index] using
grid's current velocity field: faceLeft is the flux crossing the boundary
with cell index-1, faceRight the boundary with index+1. computeRHS and
faceFluxDivergence both derive from these two faces so the divergence a
signal reads is exactly the divergence the solver integrated, not a
separately reasoned approximation of it.
*/
func (grid *FluidGrid) faceFluxPair(rho []float64, index int) (faceLeft, faceRight float64) {
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

	faceLeft = rusanovFlux1D(fluxLeft, fluxCenter, rhoLeft, rhoCenter, waveLeft)
	faceRight = rusanovFlux1D(fluxCenter, fluxRight, rhoCenter, rhoRight, waveRight)

	return faceLeft, faceRight
}

/*
faceFluxDivergence returns the signed ∇·(ρv) at rho[index]: the net rate
mass is leaving the cell through advection, positive when the cell is a
net exporter (outflow face flux exceeds inflow) and negative when it is a
net importer.
*/
func (grid *FluidGrid) faceFluxDivergence(rho []float64, index int) float64 {
	faceLeft, faceRight := grid.faceFluxPair(rho, index)

	return (faceRight - faceLeft) / grid.tickSize
}

func (grid *FluidGrid) computeRHS(
	rho []float64,
	rhs []float64,
	sources []float64,
) {
	cellCount := len(rho)
	invDxSq := 1.0 / (grid.tickSize * grid.tickSize)

	for index := 1; index < cellCount-1; index++ {
		advection := -grid.faceFluxDivergence(rho, index)

		diffusion := 0.0

		if grid.diffusionCoeff > 0 {
			diffusion = grid.diffusionCoeff *
				(rho[index+1] - 2*rho[index] + rho[index-1]) * invDxSq
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
