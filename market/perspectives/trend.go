package perspectives

/*
NewTrendPerspective builds the organic-trend playbook: the case the user described
as a coin "legitimately rising in value because of some event," as opposed to a
manufactured pump.

The thesis is the spine of DECISION.md stages 2 through 5. A genuine rise has an
authentic local driver, not macro drift — so CategoryEndogenousAlpha (Causal: the
move is caused by this symbol's own flow, not the index) is a hard gate. With that
established, executed tape has to confirm it: CategoryAggressiveDrive (CVD: net
volume and price moving together, the trend "supported by the tape") authorizes the
entry. Requiring the confirming category from each signal also excludes its
contradicting sibling for free — demanding EndogenousAlpha excludes SystemicBeta
(the drifter), demanding AggressiveDrive excludes StochasticBalance (the chop) —
because a signal emits one category at a time.

Once held the thesis is re-read continuously: CategoryActiveReversal (the book's
weight has flipped against the position) is an urgent stop, while
CategoryThermalExhaustion (the aggressive hitters are out of ammunition) harvests
the move before it rots. The reason the trade was opened decides when it is closed.
*/
func NewTrendPerspective() *strategy {
	return newStrategy(RegimeBullish, trendBranches)
}

var trendBranches = []Branch{
	snrGate(CategoryEndogenousAlpha,
		entryBranch(CategoryAggressiveDrive,
			holdingExitBranch(CategoryActiveReversal, CategoryThermalExhaustion),
		),
	),
}
