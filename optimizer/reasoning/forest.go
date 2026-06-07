package reasoning

import (
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

const strategyBreadthBonus = 0.15

/*
ForestStrategyCount is how many top-level entry roots the playbook carries.
*/
func ForestStrategyCount(forest []reasoning.Thought) int {
	count := 0

	for _, root := range forest {
		if subtreeHasEntry(root) {
			count++
		}
	}

	return count
}

/*
MergeSeedForests combines the entry roots from several single-strategy seeds and
keeps one shared protective management root.
*/
func MergeSeedForests(forests [][]reasoning.Thought) []reasoning.Thought {
	if len(forests) == 0 {
		return nil
	}

	merged := make([]reasoning.Thought, 0, len(forests)*2)
	seenManagement := false

	for _, forest := range forests {
		for _, root := range forest {
			if subtreeHasEntry(root) {
				merged = append(merged, cloneThought(root))
				continue
			}

			if seenManagement || root.Do.Type == reasoning.ActionNone {
				continue
			}

			merged = append(merged, cloneThought(root))
			seenManagement = true
		}
	}

	return merged
}
