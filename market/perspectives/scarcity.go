package perspectives

/*
NewScarcityPerspective builds the thin-market convexity playbook: scarcity gate
plus either ignition path, with exits independent of scarcity remaining hot.
*/
func NewScarcityPerspective() *strategy {
	entry := []Branch{
		snrGate(CategoryExtremeScarcity,
			entryLeaf(CategoryVerticalIgnition),
			entryLeaf(CategoryCoiledCompression),
		),
	}

	exit := holdingExitBranch(
		CategoryActiveReversal,
		CategoryFadedExhaustion,
		CategoryMechanicalCollapse,
	)

	return newStrategy(PlaybookScarcity, RegimeChoppy, EntryPolicyStandard, entry, exit)
}
