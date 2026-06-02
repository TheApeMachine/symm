package perspectives

import "slices"

/*
CanonicalPlaybookBranches rewrites a branch registry into sequential gate form:
pre-entry deny gates are nested under the entry root; exit branches stay top-level
siblings. Replay scoring and live Walk must use the same shape so the optimizer
objective matches deployment.
*/
func CanonicalPlaybookBranches(branches BranchList) BranchList {
	entryIndex := FindEntryIndex(branches)

	if entryIndex < 0 {
		return branches.Clone()
	}

	denies := make(BranchList, 0)
	exits := make(BranchList, 0)
	entry := branches[entryIndex]

	for index, branch := range branches {
		if index == entryIndex {
			continue
		}

		if branch.Observation == ObservationHolding {
			exits = append(exits, branch)

			continue
		}

		if IsTopLevelDenyGate(branch) {
			denies = append(denies, branch)

			continue
		}

		exits = append(exits, branch)
	}

	if len(denies) == 0 {
		next := make(BranchList, 0, 1+len(exits))
		next = append(next, entry)
		next = append(next, exits...)

		return next
	}

	wrappedEntry := NestDenyGatesAroundEntry(entry, denies)
	next := make(BranchList, 0, 1+len(exits))
	next = append(next, wrappedEntry)
	next = append(next, exits...)

	return next
}

/*
HasTradablePlaybook reports whether the registry contains an entry action path and
a holding exit action branch.
*/
func HasTradablePlaybook(branches BranchList) bool {
	entryIndex := FindEntryIndex(branches)

	if entryIndex < 0 {
		return false
	}

	if !BranchContainsEntryLeaf(branches[entryIndex]) {
		return false
	}

	return FindExitIndex(branches) >= 0
}

/*
HasInvalidTopLevelDenySiblings reports flat deny gates beside the entry root.
*/
func HasInvalidTopLevelDenySiblings(branches BranchList) bool {
	entryIndex := FindEntryIndex(branches)

	if entryIndex < 0 {
		return false
	}

	for index, branch := range branches {
		if index == entryIndex {
			continue
		}

		if branch.Observation == ObservationHolding {
			continue
		}

		if IsTopLevelDenyGate(branch) {
			return true
		}
	}

	return false
}

/*
IsCanonicalPlaybook reports a tradable registry without invalid top-level deny siblings.
*/
func IsCanonicalPlaybook(branches BranchList) bool {
	if !HasTradablePlaybook(branches) {
		return false
	}

	return !HasInvalidTopLevelDenySiblings(branches)
}

func FindEntryIndex(branches BranchList) int {
	for index := range branches {
		if branches[index].Observation == ObservationNotHolding {
			return index
		}

		if BranchContainsEntryLeaf(branches[index]) {
			return index
		}
	}

	return -1
}

func FindExitIndex(branches BranchList) int {
	for index := range branches {
		if branches[index].Observation != ObservationHolding {
			continue
		}

		if branches[index].Action.Type == ActionNone {
			continue
		}

		return index
	}

	return -1
}

func BranchContainsEntryLeaf(branch Branch) bool {
	if branch.Observation == ObservationNotHolding &&
		branch.Action.Type != ActionNone {
		return true
	}

	return slices.ContainsFunc(branch.Branches, BranchContainsEntryLeaf)
}

func IsTopLevelDenyGate(branch Branch) bool {
	return branch.Observation == ObservationNone &&
		branch.Action.Type == ActionNone
}

func NestDenyGatesAroundEntry(entry Branch, denies BranchList) Branch {
	wrapped := entry.Clone()

	for index := len(denies) - 1; index >= 0; index-- {
		gate := denies[index].Clone()
		gate.Branches = []Branch{wrapped}
		wrapped = gate
	}

	return wrapped
}
