package cognition

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

func lastCognition(symbol *types.Symbol) (types.Cognition, bool) {
	var cognition types.Cognition

	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		for stored := range symbol.MarketCognition(types.SourceGraph) {
			cognition = stored
		}

		if cognition.Winner != "" {
			return cognition, true
		}

		time.Sleep(time.Millisecond)
	}

	return types.Cognition{}, false
}

func TestUpdate(t *testing.T) {
	Convey("Given repeated observations around a real category transition", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		thesis := cognitionThesis(types.CategoryVerticalIgnition)
		vertical := encodeForTest(types.CategoryVerticalIgnition)
		reversal := encodeForTest(types.CategoryActiveReversal)
		exhaustion := encodeForTest(types.CategoryExhaustion)
		solver := NewSolver(
			t.Context(),
			thesis,
			tree,
			nil,
			nil,
			WithMaxSequenceLength(2),
			WithSurprisalLimit(math.Inf(1)),
		)
		defer solver.Close()

		storedSymbol, found := thesis.Symbols.Load("BTC/USD")
		So(found, ShouldBeTrue)
		symbol := storedSymbol.(*types.Symbol)

		Convey("Then observing an active category should not train before completion", func() {
			waitSequence(t, solver, "BTC/USD", []string{vertical})
			firstCount := tree.GetSensoryWeight(solver.sequenceBytes([]string{vertical})).Count

			So(solver.sequences["BTC/USD"], ShouldResemble, []string{vertical})
			So(tree.GetSensoryWeight(
				solver.sequenceBytes([]string{vertical}),
			).Count, ShouldEqual, firstCount)
			So(tree.GetSensoryWeight(
				solver.sequenceBytes([]string{vertical, vertical}),
			).Count, ShouldEqual, uint64(0))
		})

		Convey("Then only the observed category change should extend the active sequence", func() {
			waitSequence(t, solver, "BTC/USD", []string{vertical})

			symbol.Categories.Push([]types.Category{{
				Symbol:     "BTC/USD",
				Type:       types.CategoryActiveReversal,
				Confidence: 1,
				Strength:   1,
			}})
			waitSequence(t, solver, "BTC/USD", []string{vertical, reversal})
			transitionCount := tree.GetSensoryWeight(
				solver.sequenceBytes([]string{vertical, reversal}),
			).Count

			So(solver.sequences["BTC/USD"], ShouldResemble, []string{vertical, reversal})
			So(transitionCount, ShouldEqual, uint64(0))
			So(tree.GetSensoryWeight(
				solver.sequenceBytes([]string{vertical, reversal}),
			).Count, ShouldEqual, transitionCount)
		})

		Convey("Then completing the sequence should learn it exactly once", func() {
			waitSequence(t, solver, "BTC/USD", []string{vertical})

			symbol.Categories.Push([]types.Category{{
				Symbol:     "BTC/USD",
				Type:       types.CategoryActiveReversal,
				Confidence: 1,
				Strength:   1,
			}})
			waitSequence(t, solver, "BTC/USD", []string{vertical, reversal})

			symbol.Categories.Push([]types.Category{{
				Symbol:     "BTC/USD",
				Type:       types.CategoryExhaustion,
				Confidence: 1,
				Strength:   1,
			}})
			waitSequence(t, solver, "BTC/USD", []string{exhaustion})

			So(tree.GetSensoryWeight(
				solver.sequenceBytes([]string{vertical, reversal}),
			).Count, ShouldEqual, uint64(1))

			predictions := tree.PredictNextSensoryTokens(
				solver.sequenceBytes([]string{vertical}),
				make([]dmt.LookaheadPrediction, 0, 2),
			)
			So(predictions, ShouldHaveLength, 1)
			So(string(predictions[0].Token), ShouldEqual, reversal)
			So(solver.sequences["BTC/USD"], ShouldResemble, []string{exhaustion})

			cognition, found := lastCognition(symbol)
			So(found, ShouldBeTrue)
			So(cognition.Winner, ShouldEqual, "trend")
			So(cognition.WinnerClass, ShouldEqual, "trend")
			So(cognition.RegimePrefix, ShouldEqual, "trend")
			So(tree.KnownClasses(), ShouldContain, []byte("trend"))
		})
	})

	Convey("Given a tree containing an old anonymous internal concept", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		var scratch dmt.ClassificationScratch
		outcome, err := tree.ExperienceSequence([]byte("legacy_internal"), &scratch)
		So(err, ShouldBeNil)
		So(string(outcome.Class), ShouldStartWith, "concept_")
		thesis := cognitionThesis(types.CategoryVerticalIgnition)
		solver := NewSolver(t.Context(), thesis, tree, nil, nil)
		defer solver.Close()
		storedSymbol, found := thesis.Symbols.Load("BTC/USD")
		So(found, ShouldBeTrue)
		symbol := storedSymbol.(*types.Symbol)
		cognition, found := lastCognition(symbol)
		So(found, ShouldBeTrue)

		Convey("Then only the named regime-radar taxonomy crosses the Thesis boundary", func() {
			So(cognition.Winner, ShouldEqual, "trend")
			So(cognition.WinnerClass, ShouldEqual, "trend")
			So(cognition.RegimePrefix, ShouldEqual, "trend")

			for _, class := range cognition.Classes {
				So(class.Name, ShouldNotStartWith, "concept_")
			}
		})
	})

	Convey("Given a learned prefix with two measured continuations", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		thesis := cognitionThesis(types.CategoryVerticalIgnition)
		solver := NewSolver(t.Context(), thesis, tree, nil, nil)
		defer solver.Close()
		vertical := solver.encodeCategory(types.CategoryVerticalIgnition)
		reversal := solver.encodeCategory(types.CategoryActiveReversal)
		exhaustion := solver.encodeCategory(types.CategoryExhaustion)
		prefix := solver.sequenceBytes([]string{vertical})
		_, _ = tree.InsertSensoryWeight(prefix, dmt.CognitiveState{Count: 6, Probability: 1})
		_, _ = tree.InsertSensoryWeight(
			solver.sequenceBytes([]string{vertical, reversal}),
			dmt.CognitiveState{Count: 3, Probability: 0.5},
		)
		_, _ = tree.InsertSensoryWeight(
			solver.sequenceBytes([]string{vertical, exhaustion}),
			dmt.CognitiveState{Count: 3, Probability: 0.5},
		)
		storedSymbol, found := thesis.Symbols.Load("BTC/USD")
		So(found, ShouldBeTrue)
		symbol := storedSymbol.(*types.Symbol)
		waitSequence(t, solver, "BTC/USD", []string{vertical})
		cognition, found := lastCognition(symbol)
		So(found, ShouldBeTrue)

		Convey("Then it reports positive Shannon entropy and its empirical gate", func() {
			So(cognition.EntropyBits, ShouldNotBeNil)
			So(*cognition.EntropyBits, ShouldBeGreaterThan, 0)
			So(cognition.EntropyThreshold, ShouldNotBeNil)
			So(*cognition.EntropyThreshold, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given an active prefix without competing continuations", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		thesis := cognitionThesis(types.CategoryVerticalIgnition)
		solver := NewSolver(t.Context(), thesis, tree, nil, nil)
		defer solver.Close()
		storedSymbol, found := thesis.Symbols.Load("BTC/USD")
		So(found, ShouldBeTrue)
		symbol := storedSymbol.(*types.Symbol)
		waitSequence(t, solver, "BTC/USD", []string{solver.encodeCategory(types.CategoryVerticalIgnition)})
		cognition, found := lastCognition(symbol)
		So(found, ShouldBeTrue)

		Convey("Then it leaves branch entropy absent instead of publishing zero", func() {
			So(cognition.EntropyBits, ShouldBeNil)
			So(cognition.EntropyThreshold, ShouldBeNil)
		})
	})

	Convey("Given a broad thesis whose categories do not change", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		ui := make(chan []byte, 8)
		thesis := types.NewThesis(t.Context(), ui)
		thesis.At = time.Unix(1, 0).UTC()
		solver := NewSolver(t.Context(), thesis, tree, ui, nil)
		defer solver.Close()

		for index := 0; index < 129; index++ {
			symbolName := fmt.Sprintf("SYMBOL-%d/USD", index)
			symbol := types.NewSymbol(symbolName)
			symbol.Categories.Push([]types.Category{{
				Symbol:     symbolName,
				Type:       types.CategoryVerticalIgnition,
				Confidence: 1,
				Strength:   1,
			}})
			thesis.Symbols.Store(symbolName, symbol)
		}

		// Let the first observation pass settle and the publish flush.
		vertical := solver.encodeCategory(types.CategoryVerticalIgnition)
		waitSequence(t, solver, "SYMBOL-64/USD", []string{vertical})
		waitSettled(t, solver, ui)
		initialTick := solver.tickCounter

		Convey("Then unchanged observations do not retrain or publish the tree", func() {
			So(solver.tickCounter, ShouldEqual, initialTick)
			So(len(ui), ShouldEqual, 0)
		})

		stored, found := thesis.Symbols.Load("SYMBOL-64/USD")
		So(found, ShouldBeTrue)
		symbol := stored.(*types.Symbol)
		symbol.Categories.Push([]types.Category{{
			Symbol:     "SYMBOL-64/USD",
			Type:       types.CategoryActiveReversal,
			Confidence: 1,
			Strength:   1,
		}})
		waitSequence(t, solver, "SYMBOL-64/USD", []string{solver.encodeCategory(types.CategoryActiveReversal)})

		var payload struct {
			Cognition map[string]json.RawMessage `json:"cognition"`
		}
		So(json.Unmarshal(<-ui, &payload), ShouldBeNil)

		Convey("Then only the changed symbol is published without automatic REM", func() {
			So(payload.Cognition, ShouldHaveLength, 1)
			_, published := payload.Cognition["SYMBOL-64/USD"]
			So(published, ShouldBeTrue)
			var cognition types.Cognition
			for stored := range symbol.MarketCognition(types.SourceGraph) {
				cognition = stored
			}
			So(cognition.REMConsolidating, ShouldBeFalse)
			So(solver.remOutcome.ReplayedObservations, ShouldEqual, uint64(0))
		})
	})
}

/*
waitSequence polls the solver's active category sequence for a symbol until it
reaches the expected tokens, so the asynchronous self-run goroutine has advanced
the state machine to the assertion point.
*/
func waitSequence(t *testing.T, solver *Solver, symbol string, want []string) {
	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		if slicesEqual(solver.sequences[symbol], want) {
			return
		}

		time.Sleep(time.Millisecond)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}

	return true
}

/*
waitSettled pumps the run goroutine until the cognition stage has drained every
currently-available category stream: tickCounter stops advancing (no symbol is
still on its first observation) and the publish channel is flushed.
*/
func waitSettled(t *testing.T, solver *Solver, ui chan []byte) {
	deadline := time.Now().Add(3 * time.Second)
	var last uint64
	stable := 0

	for time.Now().Before(deadline) {
		select {
		case <-ui:
			last = solver.tickCounter
			stable = 0
		default:
			if solver.tickCounter == last {
				stable++
			} else {
				last = solver.tickCounter
				stable = 0
			}
		}

		if stable >= 20 {
			return
		}

		time.Sleep(time.Millisecond)
	}
}

/* encodeForTest mirrors Solver.encodeCategory for assertions before construction. */
func encodeForTest(category types.CategoryType) string {
	return new(Solver).encodeCategory(category)
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
	symbol := types.NewSymbol("BTC/USD")
	symbol.Categories.Push([]types.Category{
		{
			Symbol:     "BTC/USD",
			Type:       category,
			Confidence: 1,
			Strength:   1,
		},
		{
			Symbol:     "BTC/USD",
			Type:       types.CategoryOrganicTrend,
			Confidence: 0.8,
			Strength:   0.8,
		},
	})
	thesis.Symbols.Store("BTC/USD", symbol)

	return thesis
}

func TestPrefixTreeBranches(t *testing.T) {
	Convey("Given a trie holding divergent continuations of one prefix", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		solver := NewSolver(t.Context(), types.NewThesis(t.Context(), nil), tree, nil, nil)

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
		solver := NewSolver(t.Context(), types.NewThesis(t.Context(), nil), tree, nil, nil, WithPrefixTreeShape(1, 4, 64))

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
	solver := NewSolver(context.Background(), types.NewThesis(context.Background(), nil), tree, nil, nil)

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

func BenchmarkCognitionRun(b *testing.B) {
	tree, _ := dmt.NewTree("")
	thesis := types.NewThesis(context.Background(), nil)
	thesis.At = time.Unix(1, 0).UTC()
	solver := NewSolver(context.Background(), thesis, tree, nil, nil)
	defer solver.Close()
	rows := datura.NewMap()

	for index := range 129 {
		symbolName := fmt.Sprintf("SYMBOL-%d/USD", index)
		symbol := types.NewSymbol(symbolName)
		symbol.Categories.Push([]types.Category{{
			Symbol:     symbolName,
			Type:       types.CategoryDenseNeutrality,
			Confidence: 1,
			Strength:   1,
		}})
		thesis.Symbols.Store(symbolName, symbol)
	}

	b.ResetTimer()

	for b.Loop() {
		solver.thesis.Symbols.Range(func(key, value interface{}) bool {
			symbol, symbolOK := key.(string)
			symbolState, stateOK := value.(*types.Symbol)

			if !symbolOK || symbol == "" || !stateOK || symbolState == nil {
				return true
			}

			for batch := range symbolState.MarketCategories(types.SourceCognition) {
				_ = solver.processBatch(symbol, batch, 0, rows)
			}

			return true
		})
	}

	rows.Free()
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
				So(keys, ShouldContain, "r0")
				So(len(moved), ShouldBeGreaterThanOrEqualTo, len(first))
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

func TestStabilizeReading(t *testing.T) {
	Convey("Given a retained trend regime and a weak drive challenger", t, func() {
		solver := &Solver{readings: map[string]types.Cognition{
			"BTC/USD": {Winner: "trend", Confidence: 0.8},
		}}
		predictions := map[string]float64{"aggressive_drive": 0.7}
		reading := solver.stabilizeReading(
			"BTC/USD",
			"drive",
			0.8,
			false,
			[]types.CognitionClass{
				{Name: "drive", Probability: 0.8},
				{Name: "trend", Probability: 0.15},
			},
			predictions,
			0.95,
		)

		Convey("It should hold the incumbent and suppress lookahead evidence", func() {
			So(reading.winner, ShouldEqual, "trend")
			So(reading.confidence, ShouldEqual, 0.15)
			So(reading.predictions, ShouldBeNil)
			So(reading.held, ShouldBeTrue)
		})
	})

	Convey("Given a challenger that clears the configured switch boundary", t, func() {
		solver := &Solver{readings: map[string]types.Cognition{
			"BTC/USD": {Winner: "trend", Confidence: 0.8},
		}}
		predictions := map[string]float64{"aggressive_drive": 0.7}
		reading := solver.stabilizeReading(
			"BTC/USD",
			"drive",
			0.97,
			false,
			nil,
			predictions,
			0.95,
		)

		Convey("It should publish the new state and its current predictions", func() {
			So(reading.winner, ShouldEqual, "drive")
			So(reading.confidence, ShouldEqual, 0.97)
			So(reading.predictions, ShouldResemble, predictions)
			So(reading.held, ShouldBeFalse)
		})
	})
}
