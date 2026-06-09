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

	offset := int(math.Round((currentMid - prevMid) / grid.tickSize))
	cellCount := len(prevRho)

	for index := range grid.remappedRho {
		sourceIndex := index - offset

		if sourceIndex < 0 || sourceIndex >= cellCount {
			continue
		}

		grid.remappedRho[index] = prevRho[sourceIndex]
	}
}
