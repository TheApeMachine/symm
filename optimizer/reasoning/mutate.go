package reasoning

import "github.com/theapemachine/symm/market/perspectives"

/*
Neighbors returns the candidate forests one edit away from the given one. The
search scores them and keeps the best. The operators span the two axes the playbook
needs: DEPTH over time (temporalize turns "enter on a signal" into "see the signal,
then later when price follows through / the signal crosses up, enter" — the latched
chain Stage 4 made meaningful) and BREADTH (tighten a gate, add a parallel strategy,
add a lifecycle exit), plus numeric tuning of thresholds and protective leashes.

Every operator works on EACH entry branch, not just the first, so a multi-branch
forest can grow several deep branches independently — the shape the playbook models.
*/
func Neighbors(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	neighbors := make([][]perspectives.Thought, 0, 64)

	neighbors = append(neighbors, tuneEntryThreshold(forest, vocab)...)
	neighbors = append(neighbors, tightenEntry(forest, vocab)...)
	neighbors = append(neighbors, tightenWithVersus(forest, vocab)...)
	neighbors = append(neighbors, avoidSignal(forest, vocab)...)
	neighbors = append(neighbors, temporalizeEntry(forest, vocab)...)
	neighbors = append(neighbors, tuneManagement(forest, vocab)...)
	neighbors = append(neighbors, addLifecycleExit(forest, vocab)...)
	neighbors = append(neighbors, addTimeStop(forest, vocab)...)
	neighbors = append(neighbors, addStrategyRoot(forest, vocab)...)

	return neighbors
}

// forestHasPredicate reports whether any predicate in the forest matches.
func forestHasPredicate(forest []perspectives.Thought, match func(perspectives.Predicate) bool) bool {
	var inPredicate func(predicate perspectives.Predicate) bool
	inPredicate = func(predicate perspectives.Predicate) bool {
		if match(predicate) {
			return true
		}

		for _, operand := range predicate.All {
			if inPredicate(operand) {
				return true
			}
		}

		for _, operand := range predicate.Any {
			if inPredicate(operand) {
				return true
			}
		}

		return predicate.Not != nil && inPredicate(*predicate.Not)
	}

	var walk func(nodes []perspectives.Thought) bool
	walk = func(nodes []perspectives.Thought) bool {
		for index := range nodes {
			if inPredicate(nodes[index].When) {
				return true
			}

			if walk(nodes[index].Then) {
				return true
			}
		}

		return false
	}

	return walk(forest)
}

// ---- navigation -----------------------------------------------------------------

func subtreeHasEntry(thought perspectives.Thought) bool {
	if isEntry(thought.Do.Type) {
		return true
	}

	for _, child := range thought.Then {
		if subtreeHasEntry(child) {
			return true
		}
	}

	return false
}

// entryRootIndices returns every root whose subtree contains an entry — one per
// strategy branch. Gate and depth mutations fan out over all of them.
func entryRootIndices(forest []perspectives.Thought) []int {
	indices := make([]int, 0, len(forest))

	for index := range forest {
		if subtreeHasEntry(forest[index]) {
			indices = append(indices, index)
		}
	}

	return indices
}

func managementIndex(forest []perspectives.Thought) int {
	for index := range forest {
		action := forest[index].Do.Type
		if action != perspectives.ActionNone && !isEntry(action) {
			return index
		}
	}

	return -1
}

func signalOperandIndex(predicate perspectives.Predicate) int {
	for index, operand := range predicate.All {
		if operand.Subject == perspectives.SubjectSignal {
			return index
		}
	}

	return -1
}

func gateHasSubject(predicate perspectives.Predicate, subject perspectives.Subject) bool {
	for _, operand := range predicate.All {
		if operand.Subject == subject {
			return true
		}
	}

	return false
}

func usedEntryCategories(forest []perspectives.Thought) map[perspectives.CategoryType]bool {
	used := make(map[perspectives.CategoryType]bool)

	for index := range forest {
		for _, operand := range forest[index].When.All {
			if operand.Subject == perspectives.SubjectSignal {
				used[operand.Category] = true
			}
		}
	}

	return used
}

func hasSettleExit(forest []perspectives.Thought) bool {
	var walk func(nodes []perspectives.Thought) bool
	walk = func(nodes []perspectives.Thought) bool {
		for index := range nodes {
			if nodes[index].Do.Type == perspectives.ActionSettlePosition {
				return true
			}

			if walk(nodes[index].Then) {
				return true
			}
		}

		return false
	}

	return walk(forest)
}

func applyToManagement(
	forest []perspectives.Thought, edit func(node *perspectives.Thought),
) ([]perspectives.Thought, bool) {
	clone := cloneForest(forest)

	node, ok := managementNode(clone)
	if !ok {
		return nil, false
	}

	edit(node)

	return clone, true
}

// ---- mutations (clone, then edit the clone — parents are never aliased) ----------

// tuneEntryThreshold sweeps each branch's entry-signal SNR level across the grid.
func tuneEntryThreshold(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	neighbors := make([][]perspectives.Thought, 0)

	for _, root := range entryRootIndices(forest) {
		operand := signalOperandIndex(forest[root].When)
		if operand < 0 {
			continue
		}

		current := forest[root].When.All[operand].Value

		for _, threshold := range vocab.Thresholds {
			if threshold == current {
				continue
			}

			clone := cloneForest(forest)
			clone[root].When.All[operand].Value = threshold
			neighbors = append(neighbors, clone)
		}
	}

	return neighbors
}

// tightenEntry adds a confirming condition to each branch's gate: a regime
// requirement (if none yet) or the strongest signal it is not already watching.
func tightenEntry(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	neighbors := make([][]perspectives.Thought, 0)
	used := usedEntryCategories(forest)

	threshold := 1.0
	if len(vocab.Thresholds) > 0 {
		threshold = vocab.Thresholds[0]
	}

	for _, root := range entryRootIndices(forest) {
		gate := forest[root].When
		if gate.All == nil {
			continue
		}

		if !gateHasSubject(gate, perspectives.SubjectRegime) {
			for _, regime := range vocab.Regimes {
				clone := cloneForest(forest)
				clone[root].When.All = append(clone[root].When.All, regimeIs(regime))
				neighbors = append(neighbors, clone)
			}
		}

		for _, category := range vocab.Categories {
			if used[category] {
				continue
			}

			clone := cloneForest(forest)
			clone[root].When.All = append(clone[root].When.All, signalAtLeast(category, threshold))
			neighbors = append(neighbors, clone)

			break // one extra confirmation per step keeps the branching bounded
		}
	}

	return neighbors
}

// temporalSteps are the follow-through conditions a setup can wait for before it
// commits: price moving up by a percentage, or the signal itself crossing up.
func temporalSteps(gate perspectives.Predicate, vocab Vocabulary) []perspectives.Predicate {
	steps := make([]perspectives.Predicate, 0, len(vocab.PriceMoves)*len(vocab.Lookbacks)+len(vocab.Thresholds))

	for _, move := range vocab.PriceMoves {
		for _, ago := range vocab.Lookbacks {
			steps = append(steps, priceRoseBy(move, ago))
		}
	}

	if category, ok := gateSignalCategory(gate); ok && len(vocab.Lookbacks) > 0 {
		ago := vocab.Lookbacks[len(vocab.Lookbacks)/2]

		for _, threshold := range vocab.Thresholds {
			steps = append(steps, signalCrossedUp(category, threshold, ago))
		}
	}

	return steps
}

// temporalizeEntry pushes each branch's entry one step deeper, behind a follow-through
// the parent gate must latch and wait for — "see the setup, THEN when it confirms,
// enter". Re-applying it grows an ordered, multi-tick entry chain per branch.
func temporalizeEntry(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	neighbors := make([][]perspectives.Thought, 0)

	for _, root := range entryRootIndices(forest) {
		for _, step := range temporalSteps(forest[root].When, vocab) {
			clone := cloneForest(forest)

			node := entryNodeWithin(&clone[root])
			if node == nil {
				continue
			}

			entry := node.Do
			node.Do = perspectives.Act{}
			node.Then = append(node.Then, perspectives.Thought{When: step, Do: entry})

			neighbors = append(neighbors, clone)
		}
	}

	return neighbors
}

// tuneManagement sweeps the protective leg's type and offset.
func tuneManagement(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	index := managementIndex(forest)
	if index < 0 {
		return nil
	}

	current := forest[index].Do
	neighbors := make([][]perspectives.Thought, 0, len(vocab.Protective)*len(vocab.Offsets))

	for _, action := range vocab.Protective {
		for _, offset := range vocab.Offsets {
			if action == current.Type && offset == current.Offset {
				continue
			}

			clone, ok := applyToManagement(forest, func(node *perspectives.Thought) {
				node.Do = perspectives.Act{Type: action, Offset: offset}
			})

			if ok {
				neighbors = append(neighbors, clone)
			}
		}
	}

	return neighbors
}

// addLifecycleExit adds a trajectory-aware exit: settle once the held move has
// ended (peak rolled over), complementing the protective stop. This is the
// lifecycle management the hand-written playbook leans on.
func addLifecycleExit(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	if hasSettleExit(forest) {
		return nil
	}

	clone := cloneForest(forest)
	clone = append(clone, perspectives.Thought{
		When: allOf(holding(), lifecycleIs(perspectives.ObservationHasEnded)),
		Do:   perspectives.Act{Type: perspectives.ActionSettlePosition},
	})

	return [][]perspectives.Thought{clone}
}

// tightenWithVersus adds a metric-to-metric confirmation to each branch's gate: the
// entry signal must be stronger than another signal (e.g. the breakout dominates the
// background).
func tightenWithVersus(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	neighbors := make([][]perspectives.Thought, 0)

	for _, root := range entryRootIndices(forest) {
		gate := forest[root].When
		if gate.All == nil {
			continue
		}

		gateCategory, ok := gateSignalCategory(gate)
		if !ok {
			continue
		}

		added := 0

		for _, other := range vocab.Categories {
			if other == gateCategory {
				continue
			}

			clone := cloneForest(forest)
			clone[root].When.All = append(clone[root].When.All, signalAboveSignal(gateCategory, other))
			neighbors = append(neighbors, clone)

			added++
			if added >= 2 {
				break
			}
		}
	}

	return neighbors
}

// avoidSignal adds a negation to each branch's gate: enter only while another
// signal is absent. Which negations actually help is left to the replay score.
func avoidSignal(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	neighbors := make([][]perspectives.Thought, 0)
	used := usedEntryCategories(forest)

	threshold := 1.0
	if len(vocab.Thresholds) > 0 {
		threshold = vocab.Thresholds[0]
	}

	for _, root := range entryRootIndices(forest) {
		if forest[root].When.All == nil {
			continue
		}

		for _, other := range vocab.Categories {
			if used[other] {
				continue
			}

			clone := cloneForest(forest)
			clone[root].When.All = append(clone[root].When.All, notSignal(other, threshold))
			neighbors = append(neighbors, clone)

			break // one negation per step keeps the branching bounded
		}
	}

	return neighbors
}

// addTimeStop adds a holding time-stop: settle once the position has been open past
// a deadline, so a thesis that has not paid off is cut loose.
func addTimeStop(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	if forestHasPredicate(forest, func(predicate perspectives.Predicate) bool {
		return predicate.Subject == perspectives.SubjectElapsed
	}) {
		return nil
	}

	neighbors := make([][]perspectives.Thought, 0, len(vocab.Durations))

	for _, minutes := range vocab.Durations {
		clone := cloneForest(forest)
		clone = append(clone, perspectives.Thought{
			When: allOf(holding(), elapsedAtLeast(minutes)),
			Do:   perspectives.Act{Type: perspectives.ActionSettlePosition},
		})
		neighbors = append(neighbors, clone)
	}

	return neighbors
}

// addStrategyRoot grows the forest sideways: a fresh entry branch on a signal not
// yet traded, sharing the existing protective management.
func addStrategyRoot(forest []perspectives.Thought, vocab Vocabulary) [][]perspectives.Thought {
	used := usedEntryCategories(forest)
	neighbors := make([][]perspectives.Thought, 0)
	added := 0

	entry := perspectives.ActionMarket
	if len(vocab.Entries) > 0 {
		entry = vocab.Entries[0]
	}

	threshold := 1.0
	if len(vocab.Thresholds) > 0 {
		threshold = vocab.Thresholds[0]
	}

	for _, category := range vocab.Categories {
		if used[category] {
			continue
		}

		clone := cloneForest(forest)
		clone = append(clone, perspectives.Thought{
			When: allOf(notHolding(), signalAtLeast(category, threshold)),
			Do:   perspectives.Act{Type: entry},
		})
		neighbors = append(neighbors, clone)

		added++
		if added >= 2 {
			break
		}
	}

	return neighbors
}
