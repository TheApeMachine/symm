package scan

import (
	"math"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/cooccurrence"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/playbook"
	"github.com/theapemachine/symm/optimizer/types"
)

func (search *ScanSearch) runBeamPhase(
	phase string,
	generate func(send func(scanCandidate) bool),
) {
	log.TuneLog("scoring %s", phase)

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
			log.TuneLog(
				"finished %s early: reward stalled (%d candidates, %s)",
				phase,
				phaseCount,
				elapsed,
			)
		} else {
			log.TuneLog(
				"finished %s early: reward stalled at %.6f (%d candidates, %s)",
				phase,
				bestScore,
				phaseCount,
				elapsed,
			)
		}

		return
	}

	log.TuneLog(
		"finished %s (%d candidates, %s)",
		phase,
		phaseCount,
		elapsed,
	)
}

func (search *ScanSearch) emitBootstrapPlaybooks(
	send func(scanCandidate) bool,
	actionBranches []scanCandidate,
) {
	search.emitDecisionSeeds(send)
	search.emitSiblingBranches(send, actionBranches, search.budget.BeamWidth)
}

func (search *ScanSearch) emitDeepeningExpansions(
	send func(scanCandidate) bool,
	survivors []types.CandidateScore,
	gates []perspectives.Branch,
) {
	for _, survivor := range survivors {
		if !search.canScoreMore() {
			return
		}

		base := survivor.Branches
		reachableGates := cooccurrence.FilterReachableEntryBranchers(search.coOccurrence, base, gates)
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
	survivor types.CandidateScore,
	base perspectives.BranchList,
	gates []perspectives.Branch,
	entryCategory perspectives.CategoryType,
) bool {
	if !search.canScoreMore() {
		return false
	}

	limit := len(gates)

	if limit == 0 {
		return true
	}

	if limit > search.budget.MaxGatesPerSurvivor {
		limit = search.budget.MaxGatesPerSurvivor
	}

	for _, gate := range gates[:limit] {
		if !search.canScoreMore() {
			return false
		}

		if gate.Observation != perspectives.ObservationNone {
			continue
		}

		nested, ok := playbook.NestGateUnderEntry(base, gate)

		if !ok || playbook.BranchListsEqual(base, nested) {
			continue
		}

		search.recordNestedGateAffinity(survivor, gate, entryCategory)

		if !send(scanCandidate{
			branches: nested,
			group:    actionGroupEntry,
		}) {
			return false
		}
	}

	return true
}

func (search *ScanSearch) emitWidenExpansions(
	send func(scanCandidate) bool,
	survivors []types.CandidateScore,
	actionBranches []scanCandidate,
) {
	exits := search.groupCandidates(actionBranches, actionGroupExit)

	for _, survivor := range survivors {
		if !search.canScoreMore() {
			return
		}

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
			if !search.canScoreMore() {
				return
			}

			if len(exit.branches) == 0 {
				continue
			}

			if search.coOccurrence != nil &&
				!cooccurrence.EntryExitPairReachable(search.coOccurrence, base, exit.branches) {
				continue
			}

			widened, ok := playbook.WidenWithExit(base, exit.branches[0])

			if !ok || playbook.BranchListsEqual(base, widened) {
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
	survivor types.CandidateScore,
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
