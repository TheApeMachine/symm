package playbook

import (
	"github.com/theapemachine/symm/market/perspectives"
)

func BranchListsEqual(
	left perspectives.BranchList, right perspectives.BranchList,
) bool {
	return branchListsEqual(left, right)
}

func branchListsEqual(
	left perspectives.BranchList, right perspectives.BranchList,
) bool {
	if len(left) != len(right) {
		return false
	}

	for index := range left {
		if left[index].Category != right[index].Category ||
			left[index].Observation != right[index].Observation ||
			left[index].Value != right[index].Value {
			return false
		}
	}

	return true
}

func LastGateBranch(
	branches perspectives.BranchList,
) (perspectives.Branch, bool) {
	for index := len(branches) - 1; index >= 0; index-- {
		if branches[index].Observation == perspectives.ObservationNone {
			return branches[index], true
		}
	}

	return perspectives.Branch{}, false
}

func LastEntryChainGate(entry perspectives.Branch) (perspectives.Branch, bool) {
	current := entry

	for len(current.Branches) > 0 {
		child := current.Branches[len(current.Branches)-1]

		if child.Observation == perspectives.ObservationNotHolding &&
			child.Action.Type != perspectives.ActionNone {
			break
		}

		if child.Observation == perspectives.ObservationNone {
			current = child

			continue
		}

		break
	}

	if current.Observation == perspectives.ObservationNone {
		return current, true
	}

	return perspectives.Branch{}, false
}

func NestGateUnderEntry(
	branches perspectives.BranchList, gate perspectives.Branch,
) (perspectives.BranchList, bool) {
	entryIndex := perspectives.FindEntryIndex(branches)

	if entryIndex < 0 {
		return branches, false
	}

	next := branches.Clone()
	entry := next[entryIndex]

	if anchor, ok := LastEntryChainGate(entry); ok &&
		!IsBranchCompatible(anchor, gate) {
		return branches, false
	}

	wrapped := gate
	wrapped.Branches = []perspectives.Branch{entry}
	next[entryIndex] = wrapped

	return next, true
}

func WidenWithExit(
	base perspectives.BranchList, exit perspectives.Branch,
) (perspectives.BranchList, bool) {
	return widenWithExit(base, exit)
}

func widenWithExit(
	base perspectives.BranchList, exit perspectives.Branch,
) (perspectives.BranchList, bool) {
	entryIndex := perspectives.FindEntryIndex(base)

	if entryIndex < 0 {
		return base, false
	}

	exitIndex := perspectives.FindExitIndex(base)
	next := base.Clone()

	if exitIndex < 0 {
		next = append(next, exit)

		return next, true
	}

	next[exitIndex] = exit

	return next, true
}

func AppendEntryPathSibling(
	branches perspectives.BranchList, entry perspectives.Branch,
) perspectives.BranchList {
	next := branches.Clone()

	return append(next, entry)
}

func AppendExitSibling(
	branches perspectives.BranchList, exit perspectives.Branch,
) perspectives.BranchList {
	next := branches.Clone()

	return append(next, exit)
}

/*
ReasoningDepth is the longest nested gate chain, not sibling count.
*/
func ReasoningDepth(branches perspectives.BranchList) int {
	maxDepth := 0

	for _, branch := range branches {
		depth := 1

		if len(branch.Branches) > 0 {
			childDepth := ReasoningDepth(perspectives.BranchList(branch.Branches))
			depth += childDepth
		}

		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

/*
IsBranchCompatible rejects contradictory sequential gates.
*/
func IsBranchCompatible(
	parent perspectives.Branch, child perspectives.Branch,
) bool {
	if parent.Category == child.Category &&
		parent.Unit == perspectives.UnitSNR &&
		child.Unit == perspectives.UnitConfidence {
		return false
	}

	if parent.Category != child.Category || parent.Regime != child.Regime {
		return true
	}

	if parent.Condition == perspectives.ConditionIsGreaterThan &&
		child.Condition == perspectives.ConditionIsLessThan &&
		parent.Value > child.Value {
		return false
	}

	if parent.Condition == perspectives.ConditionIsGreaterThanOrEqual &&
		child.Condition == perspectives.ConditionIsLessThanOrEqual &&
		parent.Value > child.Value {
		return false
	}

	return true
}
