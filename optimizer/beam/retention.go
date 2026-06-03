package beam

import (
	"fmt"
	"sort"
	"strings"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/playbook"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
insertScoreBeam retains the highest-scoring candidates up to limit.
Realized PnL is the only ranking signal.
*/
func insertScoreBeam(
	top []types.CandidateScore, entry types.CandidateScore, limit int,
) []types.CandidateScore {
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

func compareBeamCandidates(left, right types.CandidateScore) bool {
	if left.AdjustedScore != right.AdjustedScore {
		return left.AdjustedScore > right.AdjustedScore
	}

	return left.Candidate < right.Candidate
}

/*
beamEligible rejects inert flat entry/exit pairs that never trade and would
otherwise score 0.000000 and fill the beam with unscored noise.
*/
func BeamEligible(entry types.CandidateScore) bool {
	return beamEligible(entry)
}

func TrainSeedEligible(entry types.CandidateScore) bool {
	return trainSeedEligible(entry)
}

func beamEligible(entry types.CandidateScore) bool {
	if entry.ClosedTrades > 0 {
		return true
	}

	if len(entry.Branches) > 2 {
		return true
	}

	if playbook.ReasoningDepth(entry.Branches) > 1 {
		return true
	}

	return false
}

/*
trainSeedEligible selects MCTS root seeds from scored beam results.
*/
func trainSeedEligible(entry types.CandidateScore) bool {
	if !beamEligible(entry) {
		return false
	}

	if perspectives.HasInvalidTopLevelDenySiblings(entry.Branches) {
		return false
	}

	return true
}

func CollapseScoreBeam(pool []types.CandidateScore, limit int) []types.CandidateScore {
	return collapseScoreBeam(pool, limit)
}

func collapseScoreBeam(pool []types.CandidateScore, limit int) []types.CandidateScore {
	deduped := DedupeCandidatesByBranch(pool)
	result := make([]types.CandidateScore, 0, limit)

	for _, candidate := range deduped {
		result = insertScoreBeam(result, candidate, limit)
	}

	return result
}

func DedupeCandidatesByBranch(pool []types.CandidateScore) []types.CandidateScore {
	seen := make(map[string]struct{}, len(pool))
	deduped := make([]types.CandidateScore, 0, len(pool))

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

/*
SelectHybridSeeds merges guard-valid top-K results with the full depth-stratified
beam so MCTS receives both profitable and structurally deep starting points.
*/
func SelectHybridSeeds(
	scored []types.CandidateScore, beam []types.CandidateScore, limit int,
) []types.CandidateScore {
	if limit <= 0 {
		return scored
	}

	pool := append(append([]types.CandidateScore(nil), scored...), beam...)

	return collapseScoreBeam(pool, limit)
}

func branchListKey(branches perspectives.BranchList) string {
	canonical := perspectives.CanonicalPlaybookBranches(branches)
	parts := make([]string, 0, len(canonical))

	for _, branch := range canonical {
		parts = append(parts, branchFingerprint(branch))
	}

	return strings.Join(parts, "|")
}

func BranchFingerprint(branch perspectives.Branch) string {
	return branchFingerprint(branch)
}

func branchFingerprint(branch perspectives.Branch) string {
	children := make([]string, 0, len(branch.Branches))

	for _, child := range branch.Branches {
		children = append(children, branchFingerprint(child))
	}

	return fmt.Sprintf(
		"%s:%d:%d:%d:%.12g:%d[%s]",
		branch.Category,
		branch.Observation,
		branch.Unit,
		branch.Condition,
		branch.Value,
		branch.Action.Type,
		strings.Join(children, ","),
	)
}
