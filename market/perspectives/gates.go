package perspectives

/*
globalEntryDenyBranches are hard blocks shared by every standard entry playbook.
Each branch terminates in ActionDeny when its category clears the noise floor.
*/
func globalEntryDenyBranches() []Branch {
	return []Branch{
		snrBranch(CategoryToxicBluff, ActionDeny),
		snrBranch(CategorySaturation, ActionDeny),
		snrBranch(CategoryTurbulent, ActionDeny),
		snrBranch(CategoryLiquidityShock, ActionDeny),
		snrBranch(CategoryMechanicalCollapse, ActionDeny),
	}
}

/*
standardEntryDenyBranches extend the global set for conviction playbooks (trend,
leadlag, scarcity): systemic beta and herd reads are passengers, not entries.
*/
func standardEntryDenyBranches() []Branch {
	denies := globalEntryDenyBranches()
	denies = append(denies,
		snrBranch(CategorySystemicBeta, ActionDeny),
		snrBranch(CategorySystemicHerd, ActionDeny),
		snrBranch(CategorySpoofTrap, ActionDeny),
		snrBranch(CategorySystemicSlump, ActionWait),
	)

	return denies
}

/*
driveEntryDenyBranches keep drive fast but still refuse manipulation and overheating.
*/
func driveEntryDenyBranches() []Branch {
	denies := globalEntryDenyBranches()
	denies = append(denies,
		snrBranch(CategorySystemicBeta, ActionDeny),
		snrBranch(CategorySpoofTrap, ActionDeny),
	)

	return denies
}

/*
pumpEntryDenyBranches allow spoof-as-entry but still block toxic bluff and overheating.
*/
func pumpEntryDenyBranches() []Branch {
	return []Branch{
		snrBranch(CategoryToxicBluff, ActionDeny),
		snrBranch(CategorySaturation, ActionDeny),
		snrBranch(CategoryTurbulent, ActionDeny),
		snrBranch(CategoryLiquidityShock, ActionDeny),
		snrBranch(CategoryMechanicalCollapse, ActionDeny),
	}
}

func denyTreeFor(policy EntryPolicy) *Tree {
	switch policy {
	case EntryPolicyPump:
		return &Tree{Branches: pumpEntryDenyBranches()}
	case EntryPolicyDrive:
		return &Tree{Branches: driveEntryDenyBranches()}
	default:
		return &Tree{Branches: standardEntryDenyBranches()}
	}
}

/*
universalExitBranches fire for any open position when exhaust or panic categories
clear the floor. They do not depend on entry gates still being hot.
*/
// UniversalExitBranches is the shared exit overlay for any open position.
func UniversalExitBranches() []Branch {
	return []Branch{
		{
			Observation: ObservationHolding,
			Branches: []Branch{
				snrBranch(CategoryActiveReversal, ActionStopLoss),
				snrBranch(CategoryMechanicalCollapse, ActionStopLoss),
				snrBranch(CategoryLiquidityShock, ActionStopLoss),
				snrBranch(CategorySaturation, ActionTakeProfit),
				snrBranch(CategoryFragileExpansion, ActionTakeProfit),
			},
		},
	}
}
