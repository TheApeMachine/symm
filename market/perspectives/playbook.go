package perspectives

import "slices"

/*
CanonicalPlaybookBranches rewrites a branch registry into sequential gate form:
pre-entry deny gates are nested under the entry root; exit branches stay top-level
siblings. Replay scoring and live Walk must use the same shape so the optimizer
objective matches deployment.

Multiple entry roots are preserved as siblings: a combined playbook holds several
distinct strategies, each its own gated entry subtree, evaluated independently by
BranchEvaluator (deepest reachable action wins per tick). A single entry collapses
to the legacy shape, so this is backward compatible.
*/
func CanonicalPlaybookBranches(branches BranchList) BranchList {
	entryIndices := FindAllEntryIndices(branches)

	if len(entryIndices) == 0 {
		return branches.Clone()
	}

	isEntry := make(map[int]struct{}, len(entryIndices))

	for _, index := range entryIndices {
		isEntry[index] = struct{}{}
	}

	entries := make(BranchList, 0, len(entryIndices))
	denies := make(BranchList, 0)
	exits := make(BranchList, 0)

	for index, branch := range branches {
		if _, ok := isEntry[index]; ok {
			entries = append(entries, branch.Clone())

			continue
		}

		if branch.Observation == ObservationHolding {
			exits = append(exits, branch.Clone())

			continue
		}

		if IsTopLevelDenyGate(branch) {
			denies = append(denies, branch.Clone())

			continue
		}

		exits = append(exits, branch.Clone())
	}

	// Legacy flat construction leaves deny gates as top-level siblings; nest them
	// under the first entry so live Walk and replay see one gated path. Combined
	// trees already carry their denies nested per strategy, so denies is empty.
	if len(denies) > 0 {
		entries[0] = NestDenyGatesAroundEntry(entries[0], denies)
	}

	next := make(BranchList, 0, len(entries)+len(exits))
	next = append(next, entries...)
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
HasInvalidTopLevelDenySiblings reports flat deny gates beside the entry roots.
Every entry subtree is skipped (a deny gate that wraps an entry is a legitimate
strategy root, not a stray sibling), so only true ungated denies are flagged.
*/
func HasInvalidTopLevelDenySiblings(branches BranchList) bool {
	entryIndices := FindAllEntryIndices(branches)

	if len(entryIndices) == 0 {
		return false
	}

	isEntry := make(map[int]struct{}, len(entryIndices))

	for _, index := range entryIndices {
		isEntry[index] = struct{}{}
	}

	for index, branch := range branches {
		if _, ok := isEntry[index]; ok {
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

/*
FindAllEntryIndices returns every top-level entry root: a not-holding branch or
any branch whose subtree reaches an entry leaf (a deny gate wrapping an entry).
Used to keep multiple distinct strategies as siblings during canonicalization.
*/
func FindAllEntryIndices(branches BranchList) []int {
	indices := make([]int, 0)

	for index := range branches {
		if branches[index].Observation == ObservationNotHolding ||
			BranchContainsEntryLeaf(branches[index]) {
			indices = append(indices, index)
		}
	}

	return indices
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
