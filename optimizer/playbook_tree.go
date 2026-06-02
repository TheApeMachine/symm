package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

func survivorGroup(survivor CandidateScore) actionGroup {
	if perspectives.FindEntryIndex(survivor.Branches) >= 0 {
		return actionGroupEntry
	}

	return actionGroupExit
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

func lastGateBranch(
	branches perspectives.BranchList,
) (perspectives.Branch, bool) {
	for index := len(branches) - 1; index >= 0; index-- {
		if branches[index].Observation == perspectives.ObservationNone {
			return branches[index], true
		}
	}

	return perspectives.Branch{}, false
}

func lastEntryChainGate(entry perspectives.Branch) (perspectives.Branch, bool) {
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

func nestGateUnderEntry(
	branches perspectives.BranchList, gate perspectives.Branch,
) (perspectives.BranchList, bool) {
	entryIndex := perspectives.FindEntryIndex(branches)

	if entryIndex < 0 {
		return branches, false
	}

	next := branches.Clone()
	entry := next[entryIndex]

	if anchor, ok := lastEntryChainGate(entry); ok &&
		!isBranchCompatible(anchor, gate) {
		return branches, false
	}

	wrapped := gate
	wrapped.Branches = []perspectives.Branch{entry}
	next[entryIndex] = wrapped

	return next, true
}

func appendEntryPathSibling(
	branches perspectives.BranchList, entry perspectives.Branch,
) perspectives.BranchList {
	next := branches.Clone()

	return append(next, entry)
}

func appendExitSibling(
	branches perspectives.BranchList, exit perspectives.Branch,
) perspectives.BranchList {
	next := branches.Clone()

	return append(next, exit)
}
