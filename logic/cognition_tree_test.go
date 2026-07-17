package logic

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
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

func TestCognitionVisualization(t *testing.T) {
	Convey("Given a trained sensory sequence", t, func() {
		tree := dmt.NewTree("")
		sequence := []byte("symbol-btc-usd_pressure-positive")
		parts := []string{"symbol-btc-usd", "pressure-positive"}
		parent := []byte("symbol-btc-usd")
		analyzer := &Analyzer{tree: tree}

		tree.TrainSensorySequence(sequence)

		var scratch dmt.ClassificationScratch
		classification := tree.Classify(sequence, &scratch)
		predictions := tree.PredictNextSensoryTokens(parent, nil)

		branches, beams, classes, beamWidth, maxHops, nodeCount, lookaheadScore, lookaheadPaths :=
			analyzer.cognitionVisualization(
				sequence, parent, parts, classification, predictions,
			)

		Convey("It should export prefix-tree branches for Cortex", func() {
			So(len(branches), ShouldBeGreaterThan, 1)
			So(nodeCount, ShouldEqual, len(branches))
			So(branches[0].Prefix, ShouldEqual, "")
			So(beamWidth, ShouldBeGreaterThan, 0)
			So(maxHops, ShouldEqual, len(parts))
		})

		Convey("It should export beam paths and basin posteriors", func() {
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
		state := manifold.State{
			Symbol: "BTC/USD", At: time.Unix(1, 0), Duration: time.Second,
			Epoch: 1, ReferencePrice: 100, InvalidReason: manifold.Valid,
			Spread: 0.01, BuyCapacity: 1000, SellCapacity: 1000,
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
			So(reading.MaxHops, ShouldBeGreaterThan, 0)
			So(reading.NodeCount, ShouldEqual, len(reading.Branches))
			So(reading.RegimePrefix, ShouldNotBeEmpty)
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

	var scratch dmt.ClassificationScratch
	classification := tree.Classify(sequence, &scratch)
	predictions := tree.PredictNextSensoryTokens(parent, nil)

	for b.Loop() {
		_, _, _, _, _, _, _, _ = analyzer.cognitionVisualization(
			sequence, parent, parts, classification, predictions,
		)
	}
}
