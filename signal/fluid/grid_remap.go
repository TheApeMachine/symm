package fluid

import "math"

const rhoFloor = 1e-12

/*
lagrangianRemap shifts the previous density field by mid-price motion so book
deltas isolate reaction terms from advection.
*/
func (grid *FluidGrid) lagrangianRemap(
	prevRho []float64,
	prevMid, currentMid float64,
) {
	grid.clearField(grid.remappedRho)
	if len(prevRho) == 0 || len(grid.remappedRho) == 0 {
		return
	}

	offset := int(math.Round((currentMid - prevMid) / grid.tickSize))
	massBefore := densityMass(prevRho)
	lastIndex := len(grid.remappedRho) - 1

	for sourceIndex, density := range prevRho {
		targetIndex := sourceIndex + offset

		if targetIndex < 0 {
			targetIndex = 0
		}

		if targetIndex > lastIndex {
			targetIndex = lastIndex
		}

		grid.remappedRho[targetIndex] += density
	}

	massAfter := densityMass(grid.remappedRho)
	if massBefore <= 0 || massAfter <= 0 {
		return
	}

	scale := massBefore / massAfter
	for index := range grid.remappedRho {
		grid.remappedRho[index] *= scale
	}
}
