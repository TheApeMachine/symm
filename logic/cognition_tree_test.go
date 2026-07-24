package logic

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
)

/*
TestCognitionVisualizationStaysBounded proves beam-search hops are not the
measurement-token cardinality. A freeze after manifold on busy ticks was
ExecuteBeamSearch(parent, width, len(parts)).
*/
func TestCognitionVisualizationStaysBounded(t *testing.T) {
	Convey("Given a trained sequence wider than the beam-search depth", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		parts := []string{"symbol-vvv-usd"}

		for index := range 64 {
			parts = append(parts, fmt.Sprintf("exhaust-metric-%d-positive", index))
		}

		sequence := []byte(strings.Join(parts, "_"))
		parent := []byte(strings.Join(parts[:len(parts)-1], "_"))
		analyzer := &Analyzer{tree: tree}
		tree.TrainSensorySequence(sequence)

		for index := range 32 {
			alternate := append([]string{}, parts...)
			alternate[len(alternate)-1] = fmt.Sprintf(
				"exhaust-metric-%d-negative", index,
			)
			tree.TrainSensorySequence([]byte(strings.Join(alternate, "_")))
		}

		var scratch dmt.ClassificationScratch
		classification, classifyErr := tree.Classify(sequence, &scratch)
		So(classifyErr, ShouldBeNil)
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
	})
}

/*
TestCognitionBranchesForkFromRoot proves Cortex exports a real radix fork, not a
sealed-bag spine with one-hop stubs. Sibling pressure tokens under the same
symbol must both appear as depth-2 children.
*/
func TestCognitionBranchesForkFromRoot(t *testing.T) {
	Convey("Given trained sequences that diverge after the symbol hop", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
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
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		analyzer := &Analyzer{tree: tree}

		for range 8 {
			tree.TrainSensorySequence([]byte(
				"symbol-btc-usd_cvd-absorption--positive_cvd-balance--positive",
			))
			tree.TrainSensorySequence([]byte(
				"symbol-eth-usd_liquidity-scarcity-score--positive_hawkes-spectral-radius--positive",
			))
		}

		btcParts := []string{"symbol-btc-usd", "cvd-absorption--positive", "cvd-balance--positive"}
		ethParts := []string{
			"symbol-eth-usd", "liquidity-scarcity-score--positive", "hawkes-spectral-radius--positive",
		}
		btcSequence := []byte(strings.Join(btcParts, "_"))
		ethSequence := []byte(strings.Join(ethParts, "_"))

		var scratch dmt.ClassificationScratch
		btcClass, err := tree.Classify(btcSequence, &scratch)
		So(err, ShouldBeNil)
		ethClass, err := tree.Classify(ethSequence, &scratch)
		So(err, ShouldBeNil)
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
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		sequence := []byte("symbol-btc-usd_pressure-positive")
		parts := []string{"symbol-btc-usd", "pressure-positive"}
		parent := []byte("symbol-btc-usd")
		analyzer := &Analyzer{tree: tree}

		tree.TrainSensorySequence(sequence)
		tree.TrainSensorySequence([]byte("symbol-eth-usd_pressure-negative"))
		tree.TrainSensorySequence([]byte("symbol-btc-usd_pressure-negative"))

		var scratch dmt.ClassificationScratch
		classification, err := tree.Classify(sequence, &scratch)
		So(err, ShouldBeNil)
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

func BenchmarkCognitionVisualization(b *testing.B) {
	tree, err := dmt.NewTree("")

	if err != nil {
		b.Fatal(err)
	}

	sequence := []byte("symbol-btc-usd_pressure-positive_divergence-negative")
	parts := []string{"symbol-btc-usd", "pressure-positive", "divergence-negative"}
	parent := []byte("symbol-btc-usd_pressure-positive")
	analyzer := &Analyzer{tree: tree}

	tree.TrainSensorySequence(sequence)
	tree.TrainSensorySequence([]byte("symbol-eth-usd_pressure-negative_divergence-positive"))
	tree.TrainSensorySequence([]byte("symbol-btc-usd_pressure-negative_divergence-positive"))

	var scratch dmt.ClassificationScratch
	classification, err := tree.Classify(sequence, &scratch)

	if err != nil {
		b.Fatal(err)
	}

	predictions := tree.PredictNextSensoryTokens(parent, nil)

	for b.Loop() {
		_, _, _, _, _, _, _, _ = analyzer.cognitionVisualization(
			sequence, parent, parts, classification, predictions,
		)
	}
}
