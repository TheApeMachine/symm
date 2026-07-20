package fluid

/*
accumulateReactionSources decomposes book deltas into add, cancel, and execute
terms after Lagrangian remapping. Trades pre-register execute volume by price.
*/
func (grid *Grid) accumulateReactionSources(currentMid float64) {
	grid.lagrangianRemap(grid.prevObservedRho, grid.prevMidPrice, currentMid)

	for index := range grid.observedRho {
		delta := grid.observedRho[index] - grid.remappedRho[index]

		if delta > 0 {
			grid.addAccumulator[index] += delta
		}

		if delta < 0 {
			removal := -delta
			executed := removal

			if executed > grid.tradeExecuteAccumulator[index] {
				executed = grid.tradeExecuteAccumulator[index]
			}

			cancelled := removal - executed

			grid.tradeExecuteAccumulator[index] -= executed
			grid.attributedExecuteAccumulator[index] += executed
			grid.cancelAccumulator[index] += cancelled
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
	index := grid.midIndex

	return grid.addAccumulator[index] - grid.cancelAccumulator[index] -
		grid.attributedExecuteAccumulator[index]
}
