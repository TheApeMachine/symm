package optimizer

import (
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
insertScoreBeam retains the highest-scoring candidates up to limit.
Realized PnL is the only ranking signal.
*/
func insertScoreBeam(
	top []CandidateScore, entry CandidateScore, limit int,
) []CandidateScore {
	if limit <= 0 {
		return top
	}

	top = append(top, entry)
	sort.Slice(top, func(leftIndex, rightIndex int) bool {
		return compareBeamCandidates(top[leftIndex], top[rightIndex])
	})

	if len(top) > limit {
		top = top[:limit]
	}

	return top
}

func compareBeamCandidates(left, right CandidateScore) bool {
	if left.AdjustedScore != right.AdjustedScore {
		return left.AdjustedScore > right.AdjustedScore
	}

	return left.Candidate < right.Candidate
}

/*
beamEligible rejects inert flat entry/exit pairs that never trade and would
otherwise score 0.000000 and fill the beam with unscored noise.
*/
func beamEligible(entry CandidateScore) bool {
	if entry.ClosedTrades > 0 {
		return true
	}

	if len(entry.Branches) > 2 {
		return true
	}

	if reasoningDepth(entry.Branches) > 1 {
		return true
	}

	return false
}

/*
trainSeedEligible selects MCTS root seeds from scored beam results.
*/
func trainSeedEligible(entry CandidateScore) bool {
	if !beamEligible(entry) {
		return false
	}

	if perspectives.HasInvalidTopLevelDenySiblings(entry.Branches) {
		return false
	}

	return true
}

func collapseScoreBeam(pool []CandidateScore, limit int) []CandidateScore {
	deduped := dedupeCandidatesByBranch(pool)
	result := make([]CandidateScore, 0, limit)

	for _, candidate := range deduped {
		result = insertScoreBeam(result, candidate, limit)
	}

	return result
}

func dedupeCandidatesByBranch(pool []CandidateScore) []CandidateScore {
	seen := make(map[string]struct{}, len(pool))
	deduped := make([]CandidateScore, 0, len(pool))

	for _, candidate := range pool {
		key := branchListKey(candidate.Branches)

		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}
		deduped = append(deduped, candidate)
	}

	return deduped
}
