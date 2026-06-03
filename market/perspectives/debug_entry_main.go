//go:build ignore

package main

import (
	"context"
	"fmt"

	"github.com/theapemachine/symm/market/perspectives"
)

func main() {
	branches := perspectives.FixturePlaybookBranches()
	canonical := perspectives.CanonicalPlaybookBranches(branches)
	entryIndex := perspectives.FindEntryIndex(canonical)

	fmt.Println("fixture entry index", entryIndex, "top branches", len(canonical))

	rows, err := perspectives.EntryPassMeasurements("BTC/EUR", 50_000, branches)

	fmt.Println("entry pass rows", rows, "err", err)

	evaluator := perspectives.NewBranchEvaluator(perspectives.BranchContext{
		Measurements: perspectives.FixturePlaybookEntryMeasurements("BTC/EUR", 50_000),
		Observations: map[perspectives.ObservationType]float64{
			perspectives.ObservationNotHolding: 1,
		},
	})
	action := evaluator.Action(canonical)

	fmt.Println("fixture action", action, "eval err", evaluator.Err())

	tree, treeErr := perspectives.NewTree(context.Background(), nil)

	if treeErr != nil {
		panic(treeErr)
	}

	fmt.Println("embedded top-level branches", len(tree.Branches()))
}
