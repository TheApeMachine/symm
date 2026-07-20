package logic

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	pmanifold "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
TestCognitionVisualizationStaysBounded proves beam-search hops are not the
measurement-token cardinality. A freeze after manifold on busy ticks was
ExecuteBeamSearch(parent, width, len(parts)).
*/
func TestCognitionVisualizationStaysBounded(t *testing.T) {
	tree := dmt.NewTree("")
	parts := make([]string, 0, 80)
	parts = append(parts, "symbol-vvv-usd")

	for index := 0; index < 64; index++ {
		parts = append(parts, fmt.Sprintf("exhaust-metric-%d-positive", index))
	}

	sequence := []byte(strings.Join(parts, "_"))
	parent := []byte(strings.Join(parts[:len(parts)-1], "_"))
	analyzer := &Analyzer{tree: tree}

	tree.TrainSensorySequence(sequence)

	for index := 0; index < 32; index++ {
		alternate := append([]string{}, parts...)
		alternate[len(alternate)-1] = fmt.Sprintf(
			"exhaust-metric-%d-negative", index,
		)
		tree.TrainSensorySequence([]byte(strings.Join(alternate, "_")))
	}

	var scratch dmt.ClassificationScratch
	classification := tree.Classify(sequence, &scratch)
	predictions := tree.PredictNextSensoryTokens(parent, nil)

	done := make(chan struct{})

	go func() {
		_, _, _, _, _, _, _, _ = analyzer.cognitionVisualization(
			sequence, parent, parts, classification, predictions,
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cognitionVisualization hung with wide measurement token set")
	}
}

/*
TestCognitionBranchesForkFromRoot proves Cortex exports a real radix fork, not a
sealed-bag spine with one-hop stubs. Sibling pressure tokens under the same
symbol must both appear as depth-2 children.
*/
func TestCognitionBranchesForkFromRoot(t *testing.T) {
	Convey("Given trained sequences that diverge after the symbol hop", t, func() {
		tree := dmt.NewTree("")
		analyzer := &Analyzer{tree: tree}

		tree.TrainSensorySequence([]byte(
			"symbol-btc-usd_pressure-positive_divergence-negative",
		))
		tree.TrainSensorySequence([]byte(
			"symbol-btc-usd_pressure-negative_divergence-positive",
		))
		tree.TrainSensorySequence([]byte(
			"symbol-eth-usd_pressure-positive_divergence-negative",
		))

		tip := tree.PredictNextSensoryTokens(
			[]byte("symbol-btc-usd_pressure-positive"),
			make([]dmt.LookaheadPrediction, 0, 4),
		)
		branches, count := analyzer.cognitionBranches(
			analyzer.treeExpandWidth(cognitionBeamWidth(tip)),
		)

		Convey("Then the root fans into multiple symbols and pressure forks", func() {
			So(count, ShouldEqual, len(branches))

			childrenOf := map[int][]string{}

			for _, branch := range branches {
				if branch.ParentID < 0 {
					continue
				}

				childrenOf[branch.ParentID] = append(
					childrenOf[branch.ParentID],
					branch.Token,
				)
			}

			So(len(childrenOf[0]), ShouldBeGreaterThanOrEqualTo, 2)

			btcID := -1

			for _, branch := range branches {
				if branch.Token == "symbol-btc-usd" {
					btcID = branch.ID
					break
				}
			}

			So(btcID, ShouldBeGreaterThanOrEqualTo, 0)
			So(len(childrenOf[btcID]), ShouldBeGreaterThanOrEqualTo, 2)
		})
	})
}

func TestCognitionBeamsStaySymbolScoped(t *testing.T) {
	Convey("Given two symbols trained on divergent sensory continuations", t, func() {
		tree := dmt.NewTree("")
		analyzer := &Analyzer{tree: tree}

		for range 8 {
			tree.TrainSensorySequence([]byte(
				"symbol-btc-usd_cvd-absorption--positive_cvd-balance--positive",
			))
			tree.TrainSensorySequence([]byte(
				"symbol-eth-usd_liquidity-scarcity-score--positive_fluid-reynolds--positive",
			))
		}

		btcParts := []string{"symbol-btc-usd", "cvd-absorption--positive", "cvd-balance--positive"}
		ethParts := []string{
			"symbol-eth-usd", "liquidity-scarcity-score--positive", "fluid-reynolds--positive",
		}
		btcSequence := []byte(strings.Join(btcParts, "_"))
		ethSequence := []byte(strings.Join(ethParts, "_"))

		var scratch dmt.ClassificationScratch
		btcClass := tree.Classify(btcSequence, &scratch)
		ethClass := tree.Classify(ethSequence, &scratch)
		btcTip := tree.PredictNextSensoryTokens([]byte("symbol-btc-usd"), nil)
		ethTip := tree.PredictNextSensoryTokens([]byte("symbol-eth-usd"), nil)

		_, btcBeams, _, _, _, _, _, _ := analyzer.cognitionVisualization(
			btcSequence, []byte("symbol-btc-usd"), btcParts, btcClass, btcTip,
		)
		_, ethBeams, _, _, _, _, _, _ := analyzer.cognitionVisualization(
			ethSequence, []byte("symbol-eth-usd"), ethParts, ethClass, ethTip,
		)

		Convey("Then each symbol's MAP beam stays inside its own namespace", func() {
			So(len(btcBeams), ShouldBeGreaterThan, 0)
			So(len(ethBeams), ShouldBeGreaterThan, 0)
			So(btcBeams[0].Sequence, ShouldContainSubstring, "symbol-btc-usd")
			So(ethBeams[0].Sequence, ShouldContainSubstring, "symbol-eth-usd")
			So(btcBeams[0].Sequence, ShouldNotEqual, ethBeams[0].Sequence)
		})
	})
}

func TestCognitionVisualization(t *testing.T) {
	Convey("Given a trained sensory sequence", t, func() {
		tree := dmt.NewTree("")
		sequence := []byte("symbol-btc-usd_pressure-positive")
		parts := []string{"symbol-btc-usd", "pressure-positive"}
		parent := []byte("symbol-btc-usd")
		analyzer := &Analyzer{tree: tree}

		tree.TrainSensorySequence(sequence)
		tree.TrainSensorySequence([]byte("symbol-eth-usd_pressure-negative"))
		tree.TrainSensorySequence([]byte("symbol-btc-usd_pressure-negative"))

		var scratch dmt.ClassificationScratch
		classification := tree.Classify(sequence, &scratch)
		predictions := tree.PredictNextSensoryTokens(parent, nil)

		branches, beams, classes, beamWidth, maxHops, nodeCount, lookaheadScore, lookaheadPaths :=
			analyzer.cognitionVisualization(
				sequence, parent, parts, classification, predictions,
			)

		Convey("It should export a bounded radix tree for Cortex", func() {
			So(len(branches), ShouldBeGreaterThan, 1)
			So(nodeCount, ShouldEqual, len(branches))
			So(branches[0].Prefix, ShouldEqual, "")
			So(beamWidth, ShouldBeGreaterThan, 0)
			So(maxHops, ShouldEqual, cognitionTreeDepth())

			maxDepth := 0

			for _, branch := range branches {
				if branch.Depth > maxDepth {
					maxDepth = branch.Depth
				}
			}

			So(maxDepth, ShouldBeLessThanOrEqualTo, cognitionTreeDepth())
		})

		Convey("It should export diverging beam paths and basin posteriors", func() {
			So(len(beams), ShouldBeGreaterThan, 0)
			So(lookaheadPaths, ShouldEqual, len(beams))
			So(lookaheadScore, ShouldBeGreaterThan, 0)
			So(classes, ShouldNotBeNil)
		})
	})
}

func TestAnalyzerCognizeExportsVisualization(t *testing.T) {
	Convey("Given repeated physical evidence for one symbol", t, func() {
		tree := dmt.NewTree("")
		analyzer := &Analyzer{
			tree:      tree,
			resonance: map[string]*Resonance{"BTC/USD": {}},
			causal:    map[string]*Causal{"BTC/USD": NewCausal("BTC/USD")},
		}
		analyzer.Focus("BTC/USD")
		state := manifold.State{
			Symbol: "BTC/USD", At: time.Unix(1, 0), Duration: time.Second,
			Epoch: 1, ReferencePrice: decimal.NewFromInt64(100), InvalidReason: manifold.Valid,
			Spread: 0.01, BuyCapacity: decimal.NewFromInt64(1000), SellCapacity: decimal.NewFromInt64(1000),
			BuyIntensity: 2, SellIntensity: 1,
			Reading: pmanifold.Reading{
				PressureGradX: 1, Divergence: -1, CoherenceMag2: 1,
				GuidanceSpeed: 1,
			},
		}

		first := types.NewThesis(nil, nil)
		first.Manifold.Store(state.Symbol, state)
		analyzer.Update(first)

		state.At = state.At.Add(time.Second)
		state.Epoch++
		second := types.NewThesis(nil, nil)
		second.Manifold.Store(state.Symbol, state)
		analyzer.Update(second)
		secondValue, found := second.Cognition.Load(state.Symbol)

		Convey("It should publish tree visualization fields on the Thesis", func() {
			So(found, ShouldBeTrue)

			reading := secondValue.(types.Cognition)
			So(reading.Ready, ShouldBeTrue)
			So(len(reading.Branches), ShouldBeGreaterThan, 0)
			So(len(reading.Beams), ShouldBeGreaterThan, 0)
			So(len(reading.Classes), ShouldBeGreaterThan, 0)
			So(reading.BeamWidth, ShouldBeGreaterThan, 0)
			So(reading.MaxHops, ShouldEqual, cognitionTreeDepth())
			So(reading.NodeCount, ShouldEqual, len(reading.Branches))
			So(reading.RegimePrefix, ShouldNotBeEmpty)
		})
	})
}

/*
TestAnalyzerRecall proves a newly focused replay can render the learned Cortex
without training the tree or presenting the cached field as a market event.
*/
func TestAnalyzerRecall(t *testing.T) {
	Convey("Given an unfocused market observation already learned by DMT", t, func() {
		tree := dmt.NewTree("")
		analyzer := &Analyzer{
			tree:      tree,
			resonance: make(map[string]*Resonance),
			causal:    make(map[string]*Causal),
			cognition: make(map[string]types.Cognition),
		}
		state := manifold.State{
			Symbol: "BTC/USD", At: time.Unix(1, 0), Duration: time.Second,
			Epoch: 1, ReferencePrice: decimal.NewFromInt64(100), InvalidReason: manifold.Valid,
			Spread: 0.01, BuyCapacity: decimal.NewFromInt64(1000), SellCapacity: decimal.NewFromInt64(1000),
			BuyIntensity: 2, SellIntensity: 1,
			Reading: pmanifold.Reading{
				PressureGradX: 1, Divergence: -1, CoherenceMag2: 1,
				GuidanceSpeed: 1,
			},
		}
		observed := types.NewThesis(nil, nil)
		observed.Manifold.Store(state.Symbol, state)
		analyzer.Update(observed)

		analyzer.Focus(state.Symbol)
		state.Replay = true
		replayed := types.NewThesis(nil, nil)
		replayed.Manifold.Store(state.Symbol, state)
		analyzer.Update(replayed)
		value, found := replayed.Cognition.Load(state.Symbol)

		Convey("Then it exports visualization without another learning cohort", func() {
			So(found, ShouldBeTrue)
			reading := value.(types.Cognition)
			So(reading.Cohort, ShouldEqual, 1)
			So(reading.Branches, ShouldNotBeEmpty)
			So(reading.Beams, ShouldNotBeEmpty)
			So(replayed.Forecasts, ShouldBeEmpty)
			So(replayed.Causal, ShouldBeEmpty)
		})
	})
}

func BenchmarkCognitionVisualization(b *testing.B) {
	tree := dmt.NewTree("")
	sequence := []byte("symbol-btc-usd_pressure-positive_divergence-negative")
	parts := []string{"symbol-btc-usd", "pressure-positive", "divergence-negative"}
	parent := []byte("symbol-btc-usd_pressure-positive")
	analyzer := &Analyzer{tree: tree}

	tree.TrainSensorySequence(sequence)
	tree.TrainSensorySequence([]byte("symbol-eth-usd_pressure-negative_divergence-positive"))
	tree.TrainSensorySequence([]byte("symbol-btc-usd_pressure-negative_divergence-positive"))

	var scratch dmt.ClassificationScratch
	classification := tree.Classify(sequence, &scratch)
	predictions := tree.PredictNextSensoryTokens(parent, nil)

	for b.Loop() {
		_, _, _, _, _, _, _, _ = analyzer.cognitionVisualization(
			sequence, parent, parts, classification, predictions,
		)
	}
}
