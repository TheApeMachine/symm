package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
runBeamPhase scores one generation of candidates and retains the top beam survivors.
*/
func (search *ScanSearch) runBeamPhase(
	generate func(send func(scanCandidate) bool),
) {
	search.beamScores = nil
	search.score(generate)
}

/*
emitBootstrapPlaybooks seeds decision templates only. Flat entry/exit pairs are
emitted separately under a hard global cap so deepening starts before ~11k flats.
*/
func (search *ScanSearch) emitBootstrapPlaybooks(
	send func(scanCandidate) bool,
	actionBranches []scanCandidate,
) {
	search.emitDecisionSeeds(send)
	search.emitSiblingBranches(send, actionBranches, DefaultBootstrapPairBudget)
}

/*
emitDeepeningExpansions adds one nested gate layer per survivor under the entry
chain. Flat sibling denies are not expanded here because they do not increase
reasoning depth and are normalized under entry at persist time.
*/
func (search *ScanSearch) emitDeepeningExpansions(
	send func(scanCandidate) bool,
	survivors []CandidateScore,
	gates []perspectives.Branch,
) {
	for _, survivor := range survivors {
		base := survivor.Branches
		reachableGates := filterReachableEntryBranchers(search.coOccurrence, base, gates)
		entryCategory := primaryEntryCategory(base)
		reachableGates = rankGatesByAffinity(
			search.pairAffinity, entryCategory, reachableGates, search.profile,
		)

		if !search.emitNestedGateExpansions(send, survivor, base, reachableGates, entryCategory) {
			return
		}
	}
}

func (search *ScanSearch) emitNestedGateExpansions(
	send func(scanCandidate) bool,
	survivor CandidateScore,
	base perspectives.BranchList,
	gates []perspectives.Branch,
	entryCategory perspectives.CategoryType,
) bool {
	limit := len(gates)

	if limit == 0 {
		return true
	}

	if limit > DefaultMaxGatesPerSurvivor {
		limit = DefaultMaxGatesPerSurvivor
	}

	for _, gate := range gates[:limit] {
		if gate.Observation != perspectives.ObservationNone {
			continue
		}

		nested, ok := nestGateUnderEntry(base, gate)

		if !ok || branchListsEqual(base, nested) {
			continue
		}

		if search.coOccurrence != nil &&
			!nestedEntryGateReachable(search.coOccurrence, base, gate) {
			continue
		}

		search.recordNestedGateAffinity(survivor, gate, entryCategory)

		if !send(scanCandidate{
			branches: nested,
			group:    survivorGroup(survivor),
		}) {
			return false
		}
	}

	return true
}

/*
emitWidenExpansions explores sibling exit alternatives for each survivor without
increasing reasoning depth. This keeps the beam wide while deepening runs deep.
*/
func (search *ScanSearch) emitWidenExpansions(
	send func(scanCandidate) bool,
	survivors []CandidateScore,
	actionBranches []scanCandidate,
) {
	exits := search.groupCandidates(actionBranches, actionGroupExit)

	for _, survivor := range survivors {
		base := survivor.Branches
		entryCategory := primaryEntryCategory(base)
		rankedExits := rankExitsByAffinity(search.pairAffinity, entryCategory, exits)
		limit := len(rankedExits)

		if limit == 0 {
			continue
		}

		if limit > DefaultMaxWidensPerSurvivor {
			limit = DefaultMaxWidensPerSurvivor
		}

		for _, exit := range rankedExits[:limit] {
			if search.coOccurrence != nil &&
				!entryExitPairReachable(search.coOccurrence, base, exit.branches) {
				continue
			}

			widened, ok := widenWithExit(base, exit.branches[0])

			if !ok || branchListsEqual(base, widened) {
				continue
			}

			if !send(scanCandidate{
				branches: widened,
				group:    actionGroupEntry,
			}) {
				return
			}
		}
	}
}

func (search *ScanSearch) recordNestedGateAffinity(
	survivor CandidateScore,
	gate perspectives.Branch,
	entryCategory perspectives.CategoryType,
) {
	if search.pairAffinity == nil {
		return
	}

	search.pairAffinity.RecordNestedGate(
		entryCategory,
		gate.Category,
		survivor.AdjustedScore,
	)
}
