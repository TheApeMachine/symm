package fluid

/*
accumulateReactionSources decomposes book deltas into add, cancel, and execute
terms after Lagrangian remapping. Trades pre-register execute volume by price.
*/
func (grid *Grid) accumulateReactionSources(currentMid float64) {
	grid.lagrangianRemap(grid.prevObservedRho, grid.lastMidPrice, currentMid)

	for index := range grid.observedRho {
		delta := grid.observedRho[index] - grid.remappedRho[index]
		executed := grid.tradeExecuteAccumulator[index]
		residual := delta + executed

		grid.attributedExecuteAccumulator[index] += executed
		grid.tradeExecuteAccumulator[index] = 0

		if residual >= 0 {
			grid.addAccumulator[index] += residual
		}

		if residual < 0 {
			grid.cancelAccumulator[index] -= residual
		}

		grid.sourceAccumulator[index] += delta
	}
}

/*
clearReactionAccumulators clears consumed reaction totals so events are
applied exactly once.
*/
func (grid *Grid) clearReactionAccumulators() {
	grid.clearField(grid.sourceAccumulator)
	grid.clearField(grid.addAccumulator)
	grid.clearField(grid.cancelAccumulator)
	grid.clearField(grid.tradeExecuteAccumulator)
	grid.clearField(grid.attributedExecuteAccumulator)
}

/*
midSourceBalance returns net near-mid reaction balance so published flow
distinguishes addition from removal.
*/
func (grid *Grid) midSourceBalance() float64 {
	return grid.sourceBalance
}
