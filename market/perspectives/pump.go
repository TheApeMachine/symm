package perspectives

/*
NewPumpPerspective builds pump entries (coiled compression or spoof trap) with
category-based exits. Trailing ratchets are enforced in the trader from peak price.
*/
func NewPumpPerspective() *strategy {
	entry := []Branch{
		entryLeaf(CategoryCoiledCompression),
		entryLeaf(CategorySpoofTrap),
	}

	exit := Branch{
		Observation: ObservationHolding,
		Branches: []Branch{
			snrBranch(CategoryActiveReversal, ActionStopLoss),
			snrBranch(CategoryVerticalIgnition, ActionShort),
			snrBranch(CategoryFadedExhaustion, ActionTakeProfit),
			snrBranch(CategoryVerticalIgnition, ActionStopLoss),
		},
	}

	return newStrategy(PlaybookPump, RegimeTrending, EntryPolicyPump, entry, exit)
}
