package perspectives

/*
NewDrivePerspective builds the executed-flow playbook with manipulation denies and
a full exit thesis (unlike the prior entry-only tree).
*/
func NewDrivePerspective() *strategy {
	entry := []Branch{
		entryLeaf(CategoryAggressiveDrive),
		entryLeaf(CategoryHiddenAbsorption),
	}

	exit := holdingExitBranch(
		CategoryActiveReversal,
		CategoryThermalExhaustion,
		CategoryMechanicalCollapse,
	)

	return newStrategy(PlaybookDrive, RegimeTrending, EntryPolicyDrive, entry, exit)
}
