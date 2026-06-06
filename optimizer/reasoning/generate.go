/*
Package reasoning is the optimizer for the Thought language: it grows candidate
reasoning forests from the data and scores them on the replay tape, searching for
the deep, multi-branch thought processes the playbook is meant to express. It is the
one search — there is no Branch fallback — and everything it produces serializes back
out through reasoning.MarshalThoughts.
*/
package reasoning

import (
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
Vocabulary is the alphabet the generator builds thoughts from. The signal
categories are derived from the data (only gate on microstructure that actually
occurs); the numeric grids are coarse starting points the search refines by trying
them and keeping what scores. Keeping the grids small bounds the branching factor.
*/
type Vocabulary struct {
	Categories []types.CategoryType   // signal categories present in the tape, most frequent first
	Regimes    []types.Regime         // regimes a gate may require
	Thresholds []float64              // signal SNR levels
	Lookbacks  []int                  // `ago` lookbacks for temporal steps
	PriceMoves []float64              // rose_by/fell_by percentages
	Offsets    []float64              // protective stop/take/trail fractions
	Fractions  []float64              // entry capital multipliers on trading.position_fraction
	Durations  []float64              // time-stop deadlines, in minutes
	Entries    []reasoning.ActionType // entry actions to seed/try
	Protective []reasoning.ActionType // protective exits a node may arm
}

// ---- leaf and node constructors -------------------------------------------------

func notHolding() reasoning.Predicate {
	return reasoning.Predicate{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationNotHolding}
}

func holding() reasoning.Predicate {
	return reasoning.Predicate{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationHolding}
}

func signalAtLeast(category types.CategoryType, threshold float64) reasoning.Predicate {
	return reasoning.Predicate{
		Subject: reasoning.SubjectSignal, Category: category, Unit: reasoning.UnitSNR,
		Op: reasoning.ComparisonAtLeast, Value: threshold,
	}
}

func regimeIs(regime types.Regime) reasoning.Predicate {
	return reasoning.Predicate{Subject: reasoning.SubjectRegime, Op: reasoning.ComparisonEquals, Regime: regime}
}

func priceRoseBy(move float64, ago int) reasoning.Predicate {
	return reasoning.Predicate{
		Subject: reasoning.SubjectPrice, Unit: reasoning.UnitPercentage,
		Ago: ago, Op: reasoning.ComparisonRoseBy, Value: move,
	}
}

func signalCrossedUp(category types.CategoryType, threshold float64, ago int) reasoning.Predicate {
	return reasoning.Predicate{
		Subject: reasoning.SubjectSignal, Category: category, Unit: reasoning.UnitSNR,
		Ago: ago, Op: reasoning.ComparisonCrossedUp, Value: threshold,
	}
}

func lifecycleIs(state types.ObservationType) reasoning.Predicate {
	return reasoning.Predicate{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: state}
}

// signalAboveSignal is a metric-to-metric gate: one signal must be stronger than
// another right now (e.g. ignition dominating compression).
func signalAboveSignal(category, versus types.CategoryType) reasoning.Predicate {
	return reasoning.Predicate{
		Subject: reasoning.SubjectSignal, Category: category, Unit: reasoning.UnitSNR,
		Op:     reasoning.ComparisonAbove,
		Versus: &reasoning.Operand{Subject: reasoning.SubjectSignal, Category: versus, Unit: reasoning.UnitSNR},
	}
}

// notSignal negates a signal: hold off while this category is firing (e.g. avoid a
// toxic regime).
func notSignal(category types.CategoryType, threshold float64) reasoning.Predicate {
	inner := signalAtLeast(category, threshold)

	return reasoning.Predicate{Not: &inner}
}

// elapsedAtLeast gates on time held in the position — the basis of a time-stop.
func elapsedAtLeast(minutes float64) reasoning.Predicate {
	return reasoning.Predicate{
		Subject: reasoning.SubjectElapsed, Unit: reasoning.UnitTimeMinutes,
		Op: reasoning.ComparisonAtLeast, Value: minutes,
	}
}

func allOf(operands ...reasoning.Predicate) reasoning.Predicate {
	return reasoning.Predicate{All: operands}
}

// ---- deep clone (mutations must never alias a parent forest) ---------------------

func clonePredicate(predicate reasoning.Predicate) reasoning.Predicate {
	clone := predicate

	if predicate.All != nil {
		clone.All = make([]reasoning.Predicate, len(predicate.All))
		for index := range predicate.All {
			clone.All[index] = clonePredicate(predicate.All[index])
		}
	}

	if predicate.Any != nil {
		clone.Any = make([]reasoning.Predicate, len(predicate.Any))
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

func cloneThought(thought reasoning.Thought) reasoning.Thought {
	clone := reasoning.Thought{When: clonePredicate(thought.When), Do: thought.Do}

	if thought.Then != nil {
		clone.Then = make([]reasoning.Thought, len(thought.Then))
		for index := range thought.Then {
			clone.Then[index] = cloneThought(thought.Then[index])
		}
	}

	return clone
}

func cloneForest(forest []reasoning.Thought) []reasoning.Thought {
	clone := make([]reasoning.Thought, len(forest))

	for index := range forest {
		clone[index] = cloneThought(forest[index])
	}

	return clone
}

// ---- seeds ----------------------------------------------------------------------

// seedStrategy is the minimal coherent playbook for one signal: enter when flat and
// the signal is present, then ride a trailing stop. The search grows it from here.
func seedStrategy(category types.CategoryType, vocab Vocabulary) []reasoning.Thought {
	entry := reasoning.ActionMarket
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

	return []reasoning.Thought{
		{When: allOf(notHolding(), signalAtLeast(category, threshold)), Do: reasoning.Act{Type: entry}},
		{When: holding(), Do: reasoning.Act{Type: reasoning.ActionTrailingStop, Offset: offset}},
	}
}

// Seeds returns one minimal strategy per derived category — the starting beam.
func Seeds(vocab Vocabulary) [][]reasoning.Thought {
	seeds := make([][]reasoning.Thought, 0, len(vocab.Categories))

	for _, category := range vocab.Categories {
		seeds = append(seeds, seedStrategy(category, vocab))
	}

	if len(seeds) == 0 { // a tape with no categorised signals still gets a price-only seed
		seeds = append(seeds, seedStrategy(types.CategoryTypeNone, vocab))
	}

	return seeds
}

// ---- forest navigation ----------------------------------------------------------

// entryNode returns a pointer to the (first) node in the forest that carries an
// entry action, plus whether one was found. Mutations that grow the entry reasoning
// operate here.
func entryNode(forest []reasoning.Thought) (*reasoning.Thought, bool) {
	var found *reasoning.Thought

	var walk func(nodes []reasoning.Thought)
	walk = func(nodes []reasoning.Thought) {
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

func isEntry(action reasoning.ActionType) bool {
	switch action {
	case reasoning.ActionMarket, reasoning.ActionLimit, reasoning.ActionIceberg:
		return true
	default:
		return false
	}
}

// entryNodeWithin finds the entry node inside ONE root's subtree (the deepest node
// carrying the entry action), so a mutation can deepen a specific branch rather than
// always the first one in the forest.
func entryNodeWithin(root *reasoning.Thought) *reasoning.Thought {
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
func gateSignalCategory(gate reasoning.Predicate) (types.CategoryType, bool) {
	index := signalOperandIndex(gate)
	if index < 0 {
		return types.CategoryTypeNone, false
	}

	return gate.All[index].Category, true
}

// managementNode returns the (first) node whose When watches the open position and
// whose action is protective — the leg the management mutations tune.
func managementNode(forest []reasoning.Thought) (*reasoning.Thought, bool) {
	for index := range forest {
		if forest[index].Do.Type != reasoning.ActionNone && !isEntry(forest[index].Do.Type) {
			return &forest[index], true
		}
	}

	return nil, false
}
