package perspectives

/*
NewTrendPerspective builds the organic-trend playbook: endogenous causal driver,
tape confirmation, thermal/fluid timing, and breadth gate (risk-on or idiosyncratic).
*/
func NewTrendPerspective() *strategy {
	timing := []Branch{
		snrGate(CategoryFrenzy, entryLeaf(CategoryAggressiveDrive)),
		snrGate(CategoryLaminar, entryLeaf(CategoryAggressiveDrive)),
		snrGate(CategoryInertial, entryLeaf(CategoryAggressiveDrive)),
		snrGate(CategoryHardSupport, entryLeaf(CategoryLoadedImbalance)),
	}

	entry := []Branch{
		breadthEntryGate(
			snrGate(CategoryEndogenousAlpha, timing...),
		),
	}

	exit := holdingExitBranch(
		CategoryActiveReversal,
		CategoryThermalExhaustion,
		CategoryFadedExhaustion,
	)

	return newStrategy(PlaybookTrend, RegimeBullish, EntryPolicyStandard, entry, exit)
}

/*
breadthEntryGate requires global or idiosyncratic lift before trend entries fire.
*/
func breadthEntryGate(children ...Branch) Branch {
	return Branch{
		Branches: []Branch{
			snrGate(CategoryRiskOnSurge, children...),
			snrGate(CategoryDivergentMove, children...),
			snrGate(CategoryDecoupledAlpha, children...),
		},
	}
}
