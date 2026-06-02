package optimizer

import (
	"sort"
)

const (
	DefaultMinBeamDepthSlots = 8
	DefaultBeamPruneFactor   = 4
)

/*
insertDepthStratifiedBeam retains top-scoring candidates while reserving slots
per reasoning depth so shallow flat pairs cannot evict deeper playbooks.
*/
func insertDepthStratifiedBeam(
	top []CandidateScore, entry CandidateScore, limit int,
) []CandidateScore {
	if limit <= 0 {
		return top
	}

	pool := append(append([]CandidateScore(nil), top...), entry)
	byDepth := bucketCandidatesByDepth(pool)
	depths := sortedDepthKeys(byDepth)

	if len(depths) == 0 {
		return nil
	}

	minPerDepth := minBeamDepthSlots(limit, len(depths))
	selected := make([]CandidateScore, 0, limit)
	used := make(map[int]struct{}, len(pool))

	for _, depth := range depths {
		take := minPerDepth

		if take > len(byDepth[depth]) {
			take = len(byDepth[depth])
		}

		for index := 0; index < take; index++ {
			candidate := byDepth[depth][index]
			selected = append(selected, candidate)
			used[candidate.Candidate] = struct{}{}
		}
	}

	sort.SliceStable(pool, func(leftIndex, rightIndex int) bool {
		return compareBeamCandidates(pool[leftIndex], pool[rightIndex])
	})

	for _, candidate := range pool {
		if len(selected) >= limit {
			break
		}

		if _, ok := used[candidate.Candidate]; ok {
			continue
		}

		selected = append(selected, candidate)
		used[candidate.Candidate] = struct{}{}
	}

	sort.Slice(selected, func(leftIndex, rightIndex int) bool {
		return compareBeamCandidates(selected[leftIndex], selected[rightIndex])
	})

	return selected
}

func compareBeamCandidates(left, right CandidateScore) bool {
	if left.ClosedTrades != right.ClosedTrades {
		return left.ClosedTrades > right.ClosedTrades
	}

	if left.ClosedTrades == 0 &&
		reasoningDepth(left.Branches) != reasoningDepth(right.Branches) {
		return reasoningDepth(left.Branches) > reasoningDepth(right.Branches)
	}

	if left.Score != right.Score {
		return left.Score > right.Score
	}

	return reasoningDepth(left.Branches) > reasoningDepth(right.Branches)
}

/*
beamEligible rejects inert flat entry/exit pairs that never trade and would
otherwise score 0.000000 and evict deeper playbooks from the beam.
*/
func beamEligible(entry CandidateScore) bool {
	if entry.ClosedTrades > 0 {
		return true
	}

	if reasoningDepth(entry.Branches) > 1 {
		return true
	}

	if len(entry.Branches) > 2 {
		return true
	}

	return false
}

func bucketCandidatesByDepth(
	pool []CandidateScore,
) map[int][]CandidateScore {
	byDepth := make(map[int][]CandidateScore)

	for _, candidate := range pool {
		depth := reasoningDepth(candidate.Branches)
		byDepth[depth] = append(byDepth[depth], candidate)
	}

	for depth := range byDepth {
		sort.Slice(byDepth[depth], func(leftIndex, rightIndex int) bool {
			return compareBeamCandidates(
				byDepth[depth][leftIndex], byDepth[depth][rightIndex],
			)
		})
	}

	return byDepth
}

func sortedDepthKeys(byDepth map[int][]CandidateScore) []int {
	depths := make([]int, 0, len(byDepth))

	for depth := range byDepth {
		depths = append(depths, depth)
	}

	sort.Slice(depths, func(leftIndex, rightIndex int) bool {
		return depths[leftIndex] > depths[rightIndex]
	})

	return depths
}

func collapseDepthStratifiedBeam(
	pool []CandidateScore, limit int,
) []CandidateScore {
	deduped := dedupeCandidatesByBranch(pool)
	result := make([]CandidateScore, 0, limit)

	for _, candidate := range deduped {
		result = insertDepthStratifiedBeam(result, candidate, limit)
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

func minBeamDepthSlots(limit, depthLevels int) int {
	if depthLevels <= 0 {
		return 0
	}

	slots := limit / (depthLevels * 2)

	if slots < DefaultMinBeamDepthSlots {
		slots = DefaultMinBeamDepthSlots
	}

	if slots*depthLevels > limit {
		slots = max(1, limit/depthLevels)
	}

	return slots
}
