package optimizer

import (
	"time"
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
runBeamPhase scores one generation of candidates and retains the top beam survivors.
*/
func (search *ScanSearch) runBeamPhase(
	phase string,
	generate func(send func(scanCandidate) bool),
) {
	TuneLog("scoring %s", phase)

	started := time.Now()
	scoredBefore := search.candidates

	search.beamScores = nil
	search.score(generate)

	phaseCount := search.candidates - scoredBefore
	elapsed := time.Since(started).Round(time.Millisecond)

	if search.haltPhaseOnStagnation &&
		search.progress != nil &&
		search.progress.Stagnant(search.options.BeamWidth) &&
		phaseCount > 0 {
		bestScore := search.progress.BestScore()

		if math.IsInf(bestScore, 0) {
			TuneLog(
				"finished %s early: reward stalled (%d candidates, %s)",
				phase,
				phaseCount,
				elapsed,
			)
		} else {
			TuneLog(
				"finished %s early: reward stalled at %.6f (%d candidates, %s)",
				phase,
				bestScore,
				phaseCount,
				elapsed,
			)
		}

		return
	}

	TuneLog(
		"finished %s (%d candidates, %s)",
		phase,
		phaseCount,
		elapsed,
	)
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
	search.emitSiblingBranches(send, actionBranches, search.budget.BeamWidth)
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

	if limit > search.budget.MaxGatesPerSurvivor {
		limit = search.budget.MaxGatesPerSurvivor
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

		if limit > search.budget.MaxWidensPerSurvivor {
			limit = search.budget.MaxWidensPerSurvivor
		}

		for _, exit := range rankedExits[:limit] {
			if len(exit.branches) == 0 {
				continue
			}

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
