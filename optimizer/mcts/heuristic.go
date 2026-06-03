package mcts

import (
	"math/rand"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/progress"
)

/*
Heuristic ranks and samples reachable moves during expansion and rollout.
*/
type Heuristic struct {
	moves            *Moves
	profile          *profile.Profile
	progress         *progress.SearchProgress
	stagnationWindow int
	rng              *rand.Rand
}

func NewHeuristic(
	moves *Moves,
	profile *profile.Profile,
	progress *progress.SearchProgress,
	stagnationWindow int,
	rng *rand.Rand,
) *Heuristic {
	return &Heuristic{
		moves:            moves,
		profile:          profile,
		progress:         progress,
		stagnationWindow: stagnationWindow,
		rng:              rng,
	}
}

func (heuristic *Heuristic) Reachable(
	candidates []Move, branches perspectives.BranchList,
) []Move {
	return heuristic.moves.reachable(candidates, branches)
}

func (heuristic *Heuristic) MoveReachable(
	move Move, branches perspectives.BranchList,
) bool {
	allowed, _, _ := heuristic.moves.moveReachability(move, branches)

	return allowed
}

func (heuristic *Heuristic) SampleRolloutMove(
	candidates []Move, branches perspectives.BranchList,
) Move {
	candidates = heuristic.activeMoves(candidates, branches)

	weighted := make([]Move, 0, len(candidates))
	weights := make([]float64, 0, len(candidates))
	total := 0.0

	for _, move := range candidates {
		if !heuristic.MoveReachable(move, branches) {
			continue
		}

		if !heuristic.moves.moveCompatible(branches, move) {
			continue
		}

		weight := heuristic.moveWeightForBranches(move, branches)

		if weight <= 0 {
			continue
		}

		weighted = append(weighted, move)
		weights = append(weights, weight)
		total += weight
	}

	if len(weighted) == 0 {
		reachable := heuristic.moves.reachable(candidates, branches)

		if len(reachable) == 0 {
			return candidates[heuristic.rng.Intn(len(candidates))]
		}

		return reachable[heuristic.rng.Intn(len(reachable))]
	}

	pick := heuristic.rng.Float64() * total

	for index, weight := range weights {
		pick -= weight

		if pick <= 0 {
			return weighted[index]
		}
	}

	return weighted[len(weighted)-1]
}

func (heuristic *Heuristic) SampleMoveIndex(
	candidates []Move, branches perspectives.BranchList,
) int {
	if len(candidates) <= 1 {
		return 0
	}

	candidateIndexes := heuristic.activeMoveIndexes(candidates, branches)

	total := 0.0
	weights := make([]float64, len(candidateIndexes))

	for weightIndex, moveIndex := range candidateIndexes {
		weight := heuristic.moveWeightForBranches(candidates[moveIndex], branches)

		if weight <= 0 {
			continue
		}

		weights[weightIndex] = weight
		total += weight
	}

	if total <= 0 {
		return candidateIndexes[heuristic.rng.Intn(len(candidateIndexes))]
	}

	pick := heuristic.rng.Float64() * total

	for weightIndex, weight := range weights {
		pick -= weight

		if pick <= 0 {
			return candidateIndexes[weightIndex]
		}
	}

	return candidateIndexes[len(candidateIndexes)-1]
}

func (heuristic *Heuristic) SampleAdversarialMove(
	candidates []Move, branches perspectives.BranchList,
) Move {
	chosen := candidates[0]
	lowestReachScore := 2.0

	for _, move := range candidates {
		reachScore := heuristic.moves.chainReachabilityScore(move, branches)

		if !move.theoretical {
			continue
		}

		if reachScore < lowestReachScore {
			lowestReachScore = reachScore
			chosen = move
		}
	}

	if lowestReachScore < 1 {
		return chosen
	}

	lowestSupport := int(^uint(0) >> 1)

	for _, move := range candidates {
		support := heuristic.moves.chainSupport(move, branches)

		if support < lowestSupport {
			lowestSupport = support
			chosen = move
		}
	}

	return chosen
}

func (heuristic *Heuristic) moveWeightForBranches(
	move Move, branches perspectives.BranchList,
) float64 {
	passes := heuristic.profile.GatePassCount(
		move.category, move.unit, move.condition, move.value,
	)

	if passes == 0 {
		return 0
	}

	categoryTotal := heuristic.profile.CategoryCount(move.category)

	if categoryTotal == 0 || heuristic.profile.Len() == 0 {
		return 0
	}

	categoryFrac := float64(categoryTotal) / float64(heuristic.profile.Len())
	passRate := float64(passes) / float64(categoryTotal)
	selectivity := gateSelectivity(passRate)

	if selectivity <= 0 {
		return 0
	}

	return categoryFrac * selectivity * categoryNovelty(branches, move.category)
}

func (heuristic *Heuristic) activeMoves(
	candidates []Move, branches perspectives.BranchList,
) []Move {
	indexes := heuristic.activeMoveIndexes(candidates, branches)
	active := make([]Move, 0, len(indexes))

	for _, moveIndex := range indexes {
		active = append(active, candidates[moveIndex])
	}

	return active
}

func (heuristic *Heuristic) activeMoveIndexes(
	candidates []Move, branches perspectives.BranchList,
) []int {
	indexes := make([]int, 0, len(candidates))

	for index := range candidates {
		indexes = append(indexes, index)
	}

	if heuristic.progress == nil ||
		!heuristic.progress.Stagnant(heuristic.stagnationWindow) {
		return indexes
	}

	deepening := make([]int, 0, len(indexes))

	for _, moveIndex := range indexes {
		if candidates[moveIndex].observation != perspectives.ObservationNone {
			continue
		}

		if perspectives.FindEntryIndex(branches) < 0 {
			continue
		}

		deepening = append(deepening, moveIndex)
	}

	if len(deepening) > 0 {
		return deepening
	}

	return indexes
}

func gateSelectivity(passRate float64) float64 {
	if passRate <= 0 || passRate >= 1 {
		return 0
	}

	return 4 * passRate * (1 - passRate)
}

func categoryNovelty(
	branches perspectives.BranchList, category perspectives.CategoryType,
) float64 {
	occurrences := countCategoryInBranches(branches, category)

	return 1 / float64(occurrences+1)
}

func countCategoryInBranches(
	branches perspectives.BranchList, category perspectives.CategoryType,
) int {
	count := 0

	for _, branch := range branches {
		if branch.Category == category {
			count++
		}

		count += countCategoryInBranches(
			perspectives.BranchList(branch.Branches),
			category,
		)
	}

	return count
}
