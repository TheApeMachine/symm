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
		tree := dmt.NewTree("")
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

		for index := range 200 {
			tree.TrainSensorySequence([]byte(fmt.Sprintf(
				"symbol-peer-%d_pressure-positive_divergence-negative_stress-positive",
				index,
			)))
		}

		var scratch dmt.ClassificationScratch
		classification := tree.Classify(sequence, &scratch)
		predictions := tree.PredictNextSensoryTokens(parent, nil)
		done := make(chan int)

		go func() {
			branches, _, _, _, _, _, _, _ := analyzer.cognitionVisualization(
				sequence, parts, classification, predictions,
			)
			done <- len(branches)
		}()

		select {
		case count := <-done:
			So(count, ShouldEqual, 4)
		case <-time.After(2 * time.Second):
			t.Fatal("cognitionVisualization hung with wide measurement token set")
		}
	})
}

/* TestAnalyzerCognitionBranches proves Cortex exports symbol-scoped radix forks. */
func TestAnalyzerCognitionBranches(t *testing.T) {
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
			[]byte("symbol-btc-usd"),
			make([]dmt.LookaheadPrediction, 0, 4),
		)
		branches, count := analyzer.cognitionBranches(
			[]byte("symbol-btc-usd"), cognitionBeamWidth(tip),
		)

		Convey("Then the root contains only the requested symbol and its pressure forks", func() {
			So(count, ShouldEqual, len(branches))
			So(branches[0].Prefix, ShouldEqual, "symbol-btc-usd")

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

			for _, branch := range branches {
				So(branch.Prefix, ShouldNotContainSubstring, "symbol-eth-usd")
			}

			So(childrenOf[0], ShouldResemble, []string{
				"pressure-negative", "pressure-positive",
			})
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
			btcSequence, btcParts, btcClass, btcTip,
		)
		_, ethBeams, _, _, _, _, _, _ := analyzer.cognitionVisualization(
			ethSequence, ethParts, ethClass, ethTip,
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
				sequence, parts, classification, predictions,
			)

		Convey("It should export a bounded radix tree for Cortex", func() {
			So(len(branches), ShouldBeGreaterThan, 1)
			So(nodeCount, ShouldEqual, len(branches))
			So(branches[0].Prefix, ShouldEqual, "symbol-btc-usd")
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
	tree := dmt.NewTree("")
	sequence := []byte("symbol-btc-usd_pressure-positive_divergence-negative")
	parts := []string{"symbol-btc-usd", "pressure-positive", "divergence-negative"}
	parent := []byte("symbol-btc-usd_pressure-positive")
	analyzer := &Analyzer{tree: tree}

	tree.TrainSensorySequence(sequence)
	tree.TrainSensorySequence([]byte("symbol-eth-usd_pressure-negative_divergence-positive"))
	tree.TrainSensorySequence([]byte("symbol-btc-usd_pressure-negative_divergence-positive"))

	for index := range 200 {
		for branch := range 9 {
			tree.TrainSensorySequence([]byte(fmt.Sprintf(
				"symbol-peer-%d_pressure-%d_divergence-negative_stress-positive",
				index, branch,
			)))
		}
	}

	var scratch dmt.ClassificationScratch
	classification := tree.Classify(sequence, &scratch)
	predictions := tree.PredictNextSensoryTokens(parent, nil)
	b.ReportAllocs()

	for b.Loop() {
		_, _, _, _, _, _, _, _ = analyzer.cognitionVisualization(
			sequence, parts, classification, predictions,
		)
	}
}
