package cognition

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

func TestUpdate(t *testing.T) {
	Convey("Given repeated observations around a real category transition", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		solver := NewSolver(
			tree,
			nil,
			nil,
			WithSurprisalLimit(math.Inf(1)),
		)
		thesis := cognitionThesis(types.CategoryVerticalIgnition)
		vertical := solver.encodeCategory(types.CategoryVerticalIgnition)
		reversal := solver.encodeCategory(types.CategoryActiveReversal)

		So(solver.Update(thesis), ShouldBeNil)
		firstCount := tree.GetSensoryWeight(solver.sequenceBytes([]string{vertical})).Count
		So(solver.Update(thesis), ShouldBeNil)

		Convey("Then a repeated category should not create or train a self-transition", func() {
			So(solver.sequences["BTC/USD"], ShouldResemble, []string{vertical})
			So(tree.GetSensoryWeight(
				solver.sequenceBytes([]string{vertical}),
			).Count, ShouldEqual, firstCount)
			So(tree.GetSensoryWeight(
				solver.sequenceBytes([]string{vertical, vertical}),
			).Count, ShouldEqual, uint64(0))
		})

		state, found := thesis.Symbols.Load("BTC/USD")
		So(found, ShouldBeTrue)
		symbol := state.(*types.Symbol)
		symbol.Categories.Store("BTC/USD", []types.Category{{
			Symbol:     "BTC/USD",
			Type:       types.CategoryActiveReversal,
			Confidence: 1,
			Strength:   1,
		}})
		So(solver.Update(thesis), ShouldBeNil)
		transitionCount := tree.GetSensoryWeight(
			solver.sequenceBytes([]string{vertical, reversal}),
		).Count
		So(solver.Update(thesis), ShouldBeNil)

		Convey("Then only the observed category change should extend the sequence", func() {
			So(solver.sequences["BTC/USD"], ShouldResemble, []string{vertical, reversal})
			So(transitionCount, ShouldEqual, uint64(1))
			So(tree.GetSensoryWeight(
				solver.sequenceBytes([]string{vertical, reversal}),
			).Count, ShouldEqual, transitionCount)

			predictions := tree.PredictNextSensoryTokens(
				solver.sequenceBytes([]string{vertical}),
				make([]dmt.LookaheadPrediction, 0, 2),
			)
			So(predictions, ShouldHaveLength, 1)
			So(string(predictions[0].Token), ShouldEqual, reversal)
		})
	})
}

func TestEncodeCategory(t *testing.T) {
	Convey("Given a category whose name contains DMT's token separator", t, func() {
		solver := &Solver{}
		encoded := solver.encodeCategory(types.CategoryVerticalIgnition)

		Convey("It should preserve the category as one DMT token", func() {
			So(encoded, ShouldNotContainSubstring, "_")
			So(strings.Split(encoded, "_"), ShouldHaveLength, 1)
			So(solver.decodeCategoryToken(encoded), ShouldEqual, string(types.CategoryVerticalIgnition))
		})
	})
}

func TestDecodeCategoryPath(t *testing.T) {
	Convey("Given an internally encoded multi-category path", t, func() {
		solver := &Solver{}
		path := solver.sequenceBytes([]string{
			solver.encodeCategory(types.CategoryVerticalIgnition),
			solver.encodeCategory(types.CategoryActiveReversal),
		})

		Convey("It should expose readable category states without internal separators", func() {
			decoded := solver.decodeCategoryPath(path)

			So(decoded, ShouldEqual, "vertical_ignition → active_reversal")
			So(decoded, ShouldNotContainSubstring, categoryTokenSeparator)
		})
	})
}

func TestFormatLookaheadPredictions(t *testing.T) {
	Convey("Given a beam whose score is a cumulative log probability", t, func() {
		solver := &Solver{}
		prefix := solver.sequenceBytes([]string{
			solver.encodeCategory(types.CategoryVerticalIgnition),
		})
		future := solver.sequenceBytes([]string{
			solver.encodeCategory(types.CategoryVerticalIgnition),
			solver.encodeCategory(types.CategoryActiveReversal),
		})
		probability := 0.37
		paths := []dmt.BeamPath{{Sequence: future, Score: math.Log(probability)}}

		Convey("It should publish the future category and exponentiated probability", func() {
			predictions := solver.formatLookaheadPredictions(paths, prefix)

			So(predictions, ShouldHaveLength, 1)
			So(predictions["active_reversal"], ShouldAlmostEqual, probability)
		})
	})
}

func cognitionThesis(category types.CategoryType) *types.Thesis {
	thesis := types.NewThesis(context.Background(), nil)
	thesis.At = time.Unix(1, 0).UTC()
	symbol := types.NewSymbol("BTC/USD", nil)
	symbol.Categories.Store("BTC/USD", []types.Category{{
		Symbol:     "BTC/USD",
		Type:       category,
		Confidence: 1,
		Strength:   1,
	}})
	thesis.Symbols.Store("BTC/USD", symbol)

	return thesis
}

func TestPrefixTreeBranches(t *testing.T) {
	Convey("Given a trie holding divergent continuations of one prefix", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		solver := NewSolver(tree, nil, nil)

		ignition := solver.encodeCategory(types.CategoryVerticalIgnition)
		reversal := solver.encodeCategory(types.CategoryActiveReversal)
		exhaust := solver.encodeCategory(types.CategoryExhaustion)

		// Two sequences share a prefix and then diverge, so the prefix has a
		// genuine sibling pair the export must reach.
		tree.TrainSensorySequence(solver.sequenceBytes([]string{ignition, reversal}))
		tree.TrainSensorySequence(solver.sequenceBytes([]string{ignition, exhaust}))

		Convey("When exporting the prefix tree for the active sequence", func() {
			branches := solver.prefixTreeBranches([]string{ignition, reversal})

			childrenOf := func(id int) []types.CognitionBranch {
				found := []types.CognitionBranch{}

				for _, branch := range branches {
					if branch.ParentID == id {
						found = append(found, branch)
					}
				}

				return found
			}

			Convey("Then the shared prefix should export both continuations", func() {
				roots := childrenOf(0)
				So(len(roots), ShouldEqual, 1)
				So(roots[0].Token, ShouldEqual, solver.decodeCategoryToken(ignition))

				// The bug this guards: a projection built from the active
				// sequence alone emits one child here and calls it a tree.
				siblings := childrenOf(roots[0].ID)
				So(len(siblings), ShouldEqual, 2)
			})

			Convey("Then every node should carry a splittable machine key", func() {
				for _, branch := range branches[1:] {
					So(branch.Key, ShouldNotEqual, "")
					So(strings.Split(branch.Key, "_"), ShouldHaveLength, branch.Depth)
				}
			})
		})
	})
}

func TestPrefixTreeBranchesPinsActivePath(t *testing.T) {
	Convey("Given an active continuation weaker than the render width allows", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		solver := NewSolver(tree, nil, nil, WithPrefixTreeShape(1, 4, 64))

		ignition := solver.encodeCategory(types.CategoryVerticalIgnition)
		reversal := solver.encodeCategory(types.CategoryActiveReversal)
		exhaust := solver.encodeCategory(types.CategoryExhaustion)

		for range 5 {
			tree.TrainSensorySequence(solver.sequenceBytes([]string{ignition, reversal}))
		}

		tree.TrainSensorySequence(solver.sequenceBytes([]string{ignition, exhaust}))

		Convey("When the weak continuation is the live one", func() {
			branches := solver.prefixTreeBranches([]string{ignition, exhaust})
			tokens := []string{}

			for _, branch := range branches {
				tokens = append(tokens, branch.Token)
			}

			Convey("Then it should still be exported so the beam has a node", func() {
				So(tokens, ShouldContain, solver.decodeCategoryToken(exhaust))
			})
		})
	})
}

/*
prefixTreeFixture builds a deliberately broad trie so the export benchmarks
measure a fan-out the walk actually has to bound.
*/
func prefixTreeFixture() (*Solver, []string) {
	tree, _ := dmt.NewTree("")
	solver := NewSolver(tree, nil, nil)

	for first := range 8 {
		for second := range 8 {
			for third := range 8 {
				tree.TrainSensorySequence(solver.sequenceBytes([]string{
					fmt.Sprintf("r%d", first),
					fmt.Sprintf("s%d", second),
					fmt.Sprintf("t%d", third),
				}))
			}
		}
	}

	return solver, []string{"r0", "s0", "t0"}
}

func BenchmarkPrefixTreeBranches(b *testing.B) {
	solver, active := prefixTreeFixture()

	for b.Loop() {
		_ = solver.prefixTreeBranches(active)
	}
}

func BenchmarkCachedPrefixTree(b *testing.B) {
	solver, active := prefixTreeFixture()
	_ = solver.cachedPrefixTree("BTC/USD", active, true)

	for b.Loop() {
		solver.tickCounter = 1
		_ = solver.cachedPrefixTree("BTC/USD", active, false)
	}
}

func TestCachedPrefixTree(t *testing.T) {
	Convey("Given an exported prefix tree held between ticks", t, func() {
		solver, active := prefixTreeFixture()

		first := solver.cachedPrefixTree("BTC/USD", active, true)

		Convey("When no transition occurs", func() {
			solver.tickCounter = 1
			again := solver.cachedPrefixTree("BTC/USD", active, false)

			Convey("Then the held walk should be reused", func() {
				So(len(again), ShouldEqual, len(first))
			})
		})

		Convey("When the sequence transitions", func() {
			solver.tickCounter = 2
			moved := solver.cachedPrefixTree("BTC/USD", []string{"r1"}, true)

			Convey("Then the walk should follow the new sequence", func() {
				keys := []string{}

				for _, branch := range moved {
					keys = append(keys, branch.Key)
				}

				So(keys, ShouldContain, "r1")
			})
		})

		Convey("When the cache ages past the refresh bound", func() {
			solver.tickCounter = branchRefreshTicks + 1
			refreshed := solver.cachedPrefixTree("BTC/USD", active, false)

			Convey("Then it should be rebuilt rather than served stale forever", func() {
				So(len(refreshed), ShouldBeGreaterThan, 0)
				So(solver.branchesStamp["BTC/USD"], ShouldEqual, solver.tickCounter)
			})
		})
	})
}
