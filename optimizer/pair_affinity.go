package optimizer

import (
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
PairAffinityIndex records replay PnL for flat entry/exit category pairs scored
during search. Large spreads between pairs at the same depth are used to rank
expansions and prune toxic exit siblings before wasting budget on them.
*/
type PairAffinityIndex struct {
	pairScores  map[string]float64
	entryBest   map[perspectives.CategoryType]float64
	exitBest    map[perspectives.CategoryType]float64
	gateNestSum map[string]float64
	gateNestN   map[string]int
}

func NewPairAffinityIndex() *PairAffinityIndex {
	return &PairAffinityIndex{
		pairScores:  make(map[string]float64),
		entryBest:   make(map[perspectives.CategoryType]float64),
		exitBest:    make(map[perspectives.CategoryType]float64),
		gateNestSum: make(map[string]float64),
		gateNestN:   make(map[string]int),
	}
}

func pairKey(
	entry perspectives.CategoryType, exit perspectives.CategoryType,
) string {
	return string(entry) + ">" + string(exit)
}

func nestKey(
	entry perspectives.CategoryType, gate perspectives.CategoryType,
) string {
	return string(entry) + "+" + string(gate)
}

/*
RecordFlatPair stores the best replay score seen for one entry/exit category pair.
*/
func (index *PairAffinityIndex) RecordFlatPair(
	entry perspectives.CategoryType,
	exit perspectives.CategoryType,
	score float64,
) {
	if entry == perspectives.CategoryTypeNone || exit == perspectives.CategoryTypeNone {
		return
	}

	key := pairKey(entry, exit)
	current, ok := index.pairScores[key]

	if !ok || score > current {
		index.pairScores[key] = score
	}

	index.recordBest(index.entryBest, entry, score)
	index.recordBest(index.exitBest, exit, score)
}

/*
RecordNestedGate stores parent survivor score when a gate is nested under entry.
*/
func (index *PairAffinityIndex) RecordNestedGate(
	entry perspectives.CategoryType,
	gate perspectives.CategoryType,
	parentScore float64,
) {
	if entry == perspectives.CategoryTypeNone || gate == perspectives.CategoryTypeNone {
		return
	}

	key := nestKey(entry, gate)
	index.gateNestSum[key] += parentScore
	index.gateNestN[key]++
}

func (index *PairAffinityIndex) recordBest(
	best map[perspectives.CategoryType]float64,
	category perspectives.CategoryType,
	score float64,
) {
	current, ok := best[category]

	if !ok || score > current {
		best[category] = score
	}
}

func (index *PairAffinityIndex) PairScore(
	entry perspectives.CategoryType, exit perspectives.CategoryType,
) (float64, bool) {
	score, ok := index.pairScores[pairKey(entry, exit)]

	return score, ok
}

func (index *PairAffinityIndex) EntryBest(
	entry perspectives.CategoryType,
) (float64, bool) {
	score, ok := index.entryBest[entry]

	return score, ok
}

func (index *PairAffinityIndex) NestPrior(
	entry perspectives.CategoryType, gate perspectives.CategoryType,
) float64 {
	count := index.gateNestN[nestKey(entry, gate)]

	if count <= 0 {
		return 0
	}

	return index.gateNestSum[nestKey(entry, gate)] / float64(count)
}

func (index *PairAffinityIndex) ExitRank(
	entry perspectives.CategoryType, exit perspectives.CategoryType,
) float64 {
	if score, ok := index.PairScore(entry, exit); ok {
		return score
	}

	if score, ok := index.exitBest[exit]; ok {
		return score
	}

	return 0
}

func flatPairCategories(
	branches perspectives.BranchList,
) (perspectives.CategoryType, perspectives.CategoryType, bool) {
	if reasoningDepth(branches) != 1 || len(branches) < 2 {
		return perspectives.CategoryTypeNone, perspectives.CategoryTypeNone, false
	}

	entryIndex := perspectives.FindEntryIndex(branches)
	exitIndex := -1

	for index := range branches {
		if branches[index].Observation == perspectives.ObservationHolding {
			exitIndex = index

			break
		}
	}

	if entryIndex < 0 || exitIndex < 0 {
		return perspectives.CategoryTypeNone, perspectives.CategoryTypeNone, false
	}

	entryCategories := entryPathCategories(branches)
	exitCategories := categoriesInBranchList(
		perspectives.BranchList{branches[exitIndex]},
	)

	if len(entryCategories) == 0 || len(exitCategories) == 0 {
		return perspectives.CategoryTypeNone, perspectives.CategoryTypeNone, false
	}

	return entryCategories[0], exitCategories[0], true
}

func rankExitsByAffinity(
	index *PairAffinityIndex,
	entry perspectives.CategoryType,
	exits []scanCandidate,
) []scanCandidate {
	if index == nil || len(exits) <= 1 {
		return exits
	}

	ranked := append([]scanCandidate(nil), exits...)
	sort.SliceStable(ranked, func(leftIndex, rightIndex int) bool {
		leftExit := exitCategory(ranked[leftIndex])
		rightExit := exitCategory(ranked[rightIndex])

		return index.ExitRank(entry, leftExit) > index.ExitRank(entry, rightExit)
	})

	return ranked
}

func rankGatesByAffinity(
	index *PairAffinityIndex,
	entry perspectives.CategoryType,
	gates []perspectives.Branch,
	profile *Profile,
) []perspectives.Branch {
	if len(gates) <= 1 {
		return gates
	}

	ranked := append([]perspectives.Branch(nil), gates...)
	sort.SliceStable(ranked, func(leftIndex, rightIndex int) bool {
		left := ranked[leftIndex]
		right := ranked[rightIndex]

		leftPrior := index.NestPrior(entry, left.Category)
		rightPrior := index.NestPrior(entry, right.Category)

		if leftPrior != rightPrior {
			return leftPrior > rightPrior
		}

		leftPasses := profile.GatePassCount(
			left.Category, left.Unit, left.Condition, left.Value,
		)
		rightPasses := profile.GatePassCount(
			right.Category, right.Unit, right.Condition, right.Value,
		)

		return leftPasses > rightPasses
	})

	return ranked
}

func exitCategory(candidate scanCandidate) perspectives.CategoryType {
	categories := categoriesInBranchList(candidate.branches)

	if len(categories) == 0 {
		return perspectives.CategoryTypeNone
	}

	return categories[0]
}

func primaryEntryCategory(branches perspectives.BranchList) perspectives.CategoryType {
	categories := entryPathCategories(branches)

	if len(categories) == 0 {
		return perspectives.CategoryTypeNone
	}

	return categories[0]
}
