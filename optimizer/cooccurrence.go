package optimizer

import (
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
CoOccurrenceIndex records which category sets appear together on one tick snapshot.
Walk can only descend when every category in a chain is present at that moment.
*/
type CoOccurrenceIndex struct {
	tickSets        []map[perspectives.CategoryType]struct{}
	minSupport      int
	proximityJitter int
}

func NewCoOccurrenceIndex(tape ReplayTape, minSupport int) *CoOccurrenceIndex {
	tickSets := make([]map[perspectives.CategoryType]struct{}, 0, tape.Len())

	for _, tick := range tape.Ticks {
		if tick.Row.Symbol == "" || len(tick.Snapshots) == 0 {
			continue
		}

		tickSets = append(tickSets, snapshotCategorySet(tick.Snapshots))
	}

	if minSupport <= 0 {
		minSupport = deriveMinChainSupport(len(tickSets))
	}

	return &CoOccurrenceIndex{
		tickSets:        tickSets,
		minSupport:      minSupport,
		proximityJitter: deriveNearMissTickJitter(len(tickSets)),
	}
}

func snapshotCategorySet(
	snapshots []perspectives.Measurement,
) map[perspectives.CategoryType]struct{} {
	categories := make(map[perspectives.CategoryType]struct{})

	for _, measurement := range snapshots {
		if measurement.Category == perspectives.CategoryTypeNone {
			continue
		}

		categories[measurement.Category] = struct{}{}
	}

	return categories
}

/*
ChainReachable reports whether every category in the chain co-exists on at least
one tick snapshot, matching Tree.Walk / BranchEvaluator depth traversal.
*/
func (index *CoOccurrenceIndex) ChainReachable(
	categories []perspectives.CategoryType,
) bool {
	return index.ChainReachabilityScore(categories) > 0
}

/*
ChainReachabilityScore returns probabilistic support in [0,1]. Hard
co-occurrence scores 1; near-miss proximity decays linearly toward zero.
*/
func (index *CoOccurrenceIndex) ChainReachabilityScore(
	categories []perspectives.CategoryType,
) float64 {
	if index == nil {
		return 1
	}

	support := index.chainSupport(categories)

	if support >= index.minSupport {
		return 1
	}

	if index.minSupport <= 0 {
		return 0
	}

	hardScore := float64(support) / float64(index.minSupport)

	if index.proximityJitter <= 0 {
		return hardScore
	}

	proximity := index.ProximityChainSupport(categories, index.proximityJitter)

	if proximity <= 0 {
		return hardScore
	}

	proxScore := float64(proximity) / float64(index.minSupport)
	blended := hardScore + (proxScore * (1 - hardScore))

	if blended > 1 {
		return 1
	}

	return blended
}

func (index *CoOccurrenceIndex) chainSupport(
	categories []perspectives.CategoryType,
) int {
	required := uniqueCategories(categories)

	if len(required) == 0 {
		return 0
	}

	support := 0

	for _, tickSet := range index.tickSets {
		if categorySetContains(tickSet, required) {
			support++
		}
	}

	return support
}

/*
CoOccur reports whether two categories ever share a tick snapshot.
*/
func (index *CoOccurrenceIndex) CoOccur(
	left perspectives.CategoryType, right perspectives.CategoryType,
) bool {
	if left == right {
		return index.categorySeen(left)
	}

	for _, tickSet := range index.tickSets {
		_, leftOK := tickSet[left]
		_, rightOK := tickSet[right]

		if leftOK && rightOK {
			return true
		}
	}

	return false
}

func (index *CoOccurrenceIndex) categorySeen(
	category perspectives.CategoryType,
) bool {
	for _, tickSet := range index.tickSets {
		if _, ok := tickSet[category]; ok {
			return true
		}
	}

	return false
}

func categorySetContains(
	tickSet map[perspectives.CategoryType]struct{},
	required []perspectives.CategoryType,
) bool {
	for _, category := range required {
		if _, ok := tickSet[category]; !ok {
			return false
		}
	}

	return true
}

func uniqueCategories(
	categories []perspectives.CategoryType,
) []perspectives.CategoryType {
	if len(categories) == 0 {
		return categories
	}

	sorted := append([]perspectives.CategoryType(nil), categories...)
	sort.Slice(sorted, func(leftIndex, rightIndex int) bool {
		return sorted[leftIndex] < sorted[rightIndex]
	})

	unique := make([]perspectives.CategoryType, 0, len(sorted))
	last := sorted[0]
	unique = append(unique, last)

	for index := 1; index < len(sorted); index++ {
		if sorted[index] == last {
			continue
		}

		last = sorted[index]
		unique = append(unique, last)
	}

	return unique
}

func categoriesInBranchList(
	branches perspectives.BranchList,
) []perspectives.CategoryType {
	categories := make([]perspectives.CategoryType, 0)

	for _, branch := range branches {
		collectBranchCategories(branch, &categories)
	}

	return uniqueCategories(categories)
}

func collectBranchCategories(
	branch perspectives.Branch, categories *[]perspectives.CategoryType,
) {
	if branch.Category != perspectives.CategoryTypeNone {
		*categories = append(*categories, branch.Category)
	}

	for _, child := range branch.Branches {
		collectBranchCategories(child, categories)
	}
}

/*
entryPathCategories returns categories on the entry decision path only.
Sibling deny and exit branches are excluded — they are evaluated independently
by BranchEvaluator and must not constrain entry deepening.
*/
func entryPathCategories(branches perspectives.BranchList) []perspectives.CategoryType {
	entryIndex := perspectives.FindEntryIndex(branches)

	if entryIndex < 0 {
		return nil
	}

	categories := make([]perspectives.CategoryType, 0)
	collectBranchCategories(branches[entryIndex], &categories)

	return uniqueCategories(categories)
}

/*
CategoriesReachable reports whether each category appears on at least one tick.
Used for exit and deny siblings that are OR branches, not sequential gates.
*/
func (index *CoOccurrenceIndex) CategoriesReachable(
	categories []perspectives.CategoryType,
) bool {
	for _, category := range uniqueCategories(categories) {
		if !index.categorySeen(category) {
			return false
		}
	}

	return len(categories) > 0
}

func filterReachableBranchers(
	index *CoOccurrenceIndex,
	anchorCategories []perspectives.CategoryType,
	branchers []perspectives.Branch,
) []perspectives.Branch {
	if index == nil {
		return branchers
	}

	reachable := make([]perspectives.Branch, 0, len(branchers))

	for _, brancher := range branchers {
		chain := append(anchorCategories, brancher.Category)

		if index.ChainReachabilityScore(chain) > 0 {
			reachable = append(reachable, brancher)
		}
	}

	return reachable
}

func filterReachableEntryBranchers(
	index *CoOccurrenceIndex,
	base perspectives.BranchList,
	branchers []perspectives.Branch,
) []perspectives.Branch {
	return filterReachableBranchers(index, entryPathCategories(base), branchers)
}

func filterReachableExitCandidates(
	index *CoOccurrenceIndex,
	candidates []scanCandidate,
) []scanCandidate {
	if index == nil {
		return candidates
	}

	reachable := make([]scanCandidate, 0, len(candidates))

	for _, candidate := range candidates {
		if index.CategoriesReachable(categoriesInBranchList(candidate.branches)) {
			reachable = append(reachable, candidate)
		}
	}

	return reachable
}

func filterReachableEntryCandidates(
	index *CoOccurrenceIndex,
	base perspectives.BranchList,
	candidates []scanCandidate,
) []scanCandidate {
	if index == nil {
		return candidates
	}

	anchor := entryPathCategories(base)
	reachable := make([]scanCandidate, 0, len(candidates))

	for _, candidate := range candidates {
		chain := append(anchor, categoriesInBranchList(candidate.branches)...)

		if index.ChainReachabilityScore(chain) > 0 {
			reachable = append(reachable, candidate)
		}
	}

	return reachable
}

func nestedEntryGateReachable(
	index *CoOccurrenceIndex,
	base perspectives.BranchList,
	gate perspectives.Branch,
) bool {
	if index == nil {
		return true
	}

	chain := append(entryPathCategories(base), gate.Category)

	return index.ChainReachabilityScore(chain) > 0
}

func entryExitPairReachable(
	index *CoOccurrenceIndex,
	entry perspectives.BranchList,
	exit perspectives.BranchList,
) bool {
	if index == nil {
		return true
	}

	if index.ChainReachabilityScore(categoriesInBranchList(entry)) <= 0 {
		return false
	}

	return index.CategoriesReachable(categoriesInBranchList(exit))
}

func exitExpansionReachable(
	index *CoOccurrenceIndex,
	base perspectives.BranchList,
	exit perspectives.BranchList,
) bool {
	if index == nil {
		return true
	}

	entryCategories := entryPathCategories(base)

	if len(entryCategories) > 0 &&
		index.ChainReachabilityScore(entryCategories) <= 0 {
		return false
	}

	return index.CategoriesReachable(categoriesInBranchList(exit))
}
