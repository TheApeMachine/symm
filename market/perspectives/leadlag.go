package perspectives

/*
NewLeadLagPerspective builds the anchor catch-up playbook with standard deny gates
and decoupled exit categories that do not require the lag reading to stay hot.
*/
func NewLeadLagPerspective() *strategy {
	entry := []Branch{
		breadthEntryGate(entryLeaf(CategoryInefficientLag)),
	}

	exit := holdingExitBranch(
		CategoryActiveReversal,
		CategoryAnchorStall,
		CategorySynchronizedDrift,
	)

	return newStrategy(PlaybookLeadLag, RegimeTrending, EntryPolicyStandard, entry, exit)
}
