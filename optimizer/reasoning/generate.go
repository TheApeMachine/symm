/*
Package reasoning is the optimizer for the Thought language: it grows candidate
reasoning forests from the data and scores them on the replay tape, searching for
the deep, multi-branch thought processes the playbook is meant to express. It is the
one search — there is no Branch fallback — and everything it produces serializes back
out through perspectives.MarshalThoughts.
*/
package reasoning

import (
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
Vocabulary is the alphabet the generator builds thoughts from. The signal
categories are derived from the data (only gate on microstructure that actually
occurs); the numeric grids are coarse starting points the search refines by trying
them and keeping what scores. Keeping the grids small bounds the branching factor.
*/
type Vocabulary struct {
	Categories []perspectives.CategoryType // signal categories present in the tape, most frequent first
	Regimes    []perspectives.Regime       // regimes a gate may require
	Thresholds []float64                   // signal SNR levels
	Lookbacks  []int                       // `ago` lookbacks for temporal steps
	PriceMoves []float64                   // rose_by/fell_by percentages
	Offsets    []float64                   // protective stop/take/trail fractions
	Entries    []perspectives.ActionType   // entry actions to seed/try
	Protective []perspectives.ActionType   // protective exits a node may arm
}

// maxSeedCategories caps how many distinct signals seed their own strategy branch,
// so a noisy tape with dozens of categories does not explode the initial beam.
const maxSeedCategories = 6

/*
DeriveVocabulary reads the categories that actually occur on the tape (by
frequency) and pairs them with sensible numeric grids. The search refines within
these; the replay score is the judge of what survives.
*/
func DeriveVocabulary(rows []perspectives.Measurement) Vocabulary {
	counts := make(map[perspectives.CategoryType]int)

	for _, row := range rows {
		if row.Category == perspectives.CategoryTypeNone {
			continue
		}

		counts[row.Category]++
	}

	categories := make([]perspectives.CategoryType, 0, len(counts))

	for category := range counts {
		categories = append(categories, category)
	}

	sort.Slice(categories, func(i, j int) bool {
		if counts[categories[i]] != counts[categories[j]] {
			return counts[categories[i]] > counts[categories[j]]
		}

		return categories[i] < categories[j] // stable, deterministic tie-break
	})

	if len(categories) > maxSeedCategories {
		categories = categories[:maxSeedCategories]
	}

	return Vocabulary{
		Categories: categories,
		Regimes: []perspectives.Regime{
			perspectives.RegimeTrending, perspectives.RegimeBullish, perspectives.RegimeChoppy,
		},
		Thresholds: []float64{1.0, 1.5, 2.0},
		Lookbacks:  []int{3, 5, 8},
		PriceMoves: []float64{0.5, 1.0, 2.0},
		Offsets:    []float64{0.01, 0.02, 0.05},
		Entries:    []perspectives.ActionType{perspectives.ActionMarket, perspectives.ActionIceberg},
		Protective: []perspectives.ActionType{
			perspectives.ActionTrailingStop, perspectives.ActionStopLoss, perspectives.ActionTakeProfit,
		},
	}
}

// ---- leaf and node constructors -------------------------------------------------

func notHolding() perspectives.Predicate {
	return perspectives.Predicate{Subject: perspectives.SubjectPosition, Op: perspectives.ComparisonEquals, Lifecycle: perspectives.ObservationNotHolding}
}

func holding() perspectives.Predicate {
	return perspectives.Predicate{Subject: perspectives.SubjectPosition, Op: perspectives.ComparisonEquals, Lifecycle: perspectives.ObservationHolding}
}

func signalAtLeast(category perspectives.CategoryType, threshold float64) perspectives.Predicate {
	return perspectives.Predicate{
		Subject: perspectives.SubjectSignal, Category: category, Unit: perspectives.UnitSNR,
		Op: perspectives.ComparisonAtLeast, Value: threshold,
	}
}

func regimeIs(regime perspectives.Regime) perspectives.Predicate {
	return perspectives.Predicate{Subject: perspectives.SubjectRegime, Op: perspectives.ComparisonEquals, Regime: regime}
}

func priceRoseBy(move float64, ago int) perspectives.Predicate {
	return perspectives.Predicate{
		Subject: perspectives.SubjectPrice, Unit: perspectives.UnitPercentage,
		Ago: ago, Op: perspectives.ComparisonRoseBy, Value: move,
	}
}

func signalCrossedUp(category perspectives.CategoryType, threshold float64, ago int) perspectives.Predicate {
	return perspectives.Predicate{
		Subject: perspectives.SubjectSignal, Category: category, Unit: perspectives.UnitSNR,
		Ago: ago, Op: perspectives.ComparisonCrossedUp, Value: threshold,
	}
}

func lifecycleIs(state perspectives.ObservationType) perspectives.Predicate {
	return perspectives.Predicate{Subject: perspectives.SubjectPosition, Op: perspectives.ComparisonEquals, Lifecycle: state}
}

func allOf(operands ...perspectives.Predicate) perspectives.Predicate {
	return perspectives.Predicate{All: operands}
}

// ---- deep clone (mutations must never alias a parent forest) ---------------------

func clonePredicate(predicate perspectives.Predicate) perspectives.Predicate {
	clone := predicate

	if predicate.All != nil {
		clone.All = make([]perspectives.Predicate, len(predicate.All))
		for index := range predicate.All {
			clone.All[index] = clonePredicate(predicate.All[index])
		}
	}

	if predicate.Any != nil {
		clone.Any = make([]perspectives.Predicate, len(predicate.Any))
		for index := range predicate.Any {
			clone.Any[index] = clonePredicate(predicate.Any[index])
		}
	}

	if predicate.Not != nil {
		inner := clonePredicate(*predicate.Not)
		clone.Not = &inner
	}

	if predicate.Versus != nil {
		operand := *predicate.Versus
		clone.Versus = &operand
	}

	return clone
}

func cloneThought(thought perspectives.Thought) perspectives.Thought {
	clone := perspectives.Thought{When: clonePredicate(thought.When), Do: thought.Do}

	if thought.Then != nil {
		clone.Then = make([]perspectives.Thought, len(thought.Then))
		for index := range thought.Then {
			clone.Then[index] = cloneThought(thought.Then[index])
		}
	}

	return clone
}

func cloneForest(forest []perspectives.Thought) []perspectives.Thought {
	clone := make([]perspectives.Thought, len(forest))

	for index := range forest {
		clone[index] = cloneThought(forest[index])
	}

	return clone
}

// ---- seeds ----------------------------------------------------------------------

// seedStrategy is the minimal coherent playbook for one signal: enter when flat and
// the signal is present, then ride a trailing stop. The search grows it from here.
func seedStrategy(category perspectives.CategoryType, vocab Vocabulary) []perspectives.Thought {
	entry := perspectives.ActionMarket
	if len(vocab.Entries) > 0 {
		entry = vocab.Entries[0]
	}

	threshold := 1.0
	if len(vocab.Thresholds) > 0 {
		threshold = vocab.Thresholds[0]
	}

	offset := 0.02
	if len(vocab.Offsets) > 0 {
		offset = vocab.Offsets[len(vocab.Offsets)/2]
	}

	return []perspectives.Thought{
		{When: allOf(notHolding(), signalAtLeast(category, threshold)), Do: perspectives.Act{Type: entry}},
		{When: holding(), Do: perspectives.Act{Type: perspectives.ActionTrailingStop, Offset: offset}},
	}
}

// Seeds returns one minimal strategy per derived category — the starting beam.
func Seeds(vocab Vocabulary) [][]perspectives.Thought {
	seeds := make([][]perspectives.Thought, 0, len(vocab.Categories))

	for _, category := range vocab.Categories {
		seeds = append(seeds, seedStrategy(category, vocab))
	}

	if len(seeds) == 0 { // a tape with no categorised signals still gets a price-only seed
		seeds = append(seeds, seedStrategy(perspectives.CategoryTypeNone, vocab))
	}

	return seeds
}

// ---- forest navigation ----------------------------------------------------------

// entryNode returns a pointer to the (first) node in the forest that carries an
// entry action, plus whether one was found. Mutations that grow the entry reasoning
// operate here.
func entryNode(forest []perspectives.Thought) (*perspectives.Thought, bool) {
	var found *perspectives.Thought

	var walk func(nodes []perspectives.Thought)
	walk = func(nodes []perspectives.Thought) {
		for index := range nodes {
			if found != nil {
				return
			}

			if isEntry(nodes[index].Do.Type) {
				found = &nodes[index]
				return
			}

			walk(nodes[index].Then)
		}
	}

	walk(forest)

	return found, found != nil
}

func isEntry(action perspectives.ActionType) bool {
	switch action {
	case perspectives.ActionMarket, perspectives.ActionLimit, perspectives.ActionIceberg:
		return true
	default:
		return false
	}
}

// entryNodeWithin finds the entry node inside ONE root's subtree (the deepest node
// carrying the entry action), so a mutation can deepen a specific branch rather than
// always the first one in the forest.
func entryNodeWithin(root *perspectives.Thought) *perspectives.Thought {
	if isEntry(root.Do.Type) {
		return root
	}

	for index := range root.Then {
		if found := entryNodeWithin(&root.Then[index]); found != nil {
			return found
		}
	}

	return nil
}

// gateSignalCategory returns the signal category an All gate keys on, if any.
func gateSignalCategory(gate perspectives.Predicate) (perspectives.CategoryType, bool) {
	index := signalOperandIndex(gate)
	if index < 0 {
		return perspectives.CategoryTypeNone, false
	}

	return gate.All[index].Category, true
}

// managementNode returns the (first) node whose When watches the open position and
// whose action is protective — the leg the management mutations tune.
func managementNode(forest []perspectives.Thought) (*perspectives.Thought, bool) {
	for index := range forest {
		if forest[index].Do.Type != perspectives.ActionNone && !isEntry(forest[index].Do.Type) {
			return &forest[index], true
		}
	}

	return nil, false
}
