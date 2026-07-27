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
		parts := []string{"s"}

		for index := range 64 {
			parts = append(parts, fmt.Sprintf("category-%d-positive", index))
		}

		sequence := []byte(strings.Join(parts, "_"))
		parent := []byte(strings.Join(parts[:len(parts)-1], "_"))
		analyzer := &Analyzer{tree: tree}
		tree.TrainSensorySequence(sequence)

		for index := range 32 {
			alternate := append([]string{}, parts...)
			alternate[len(alternate)-1] = fmt.Sprintf("category-%d-negative", index)
			tree.TrainSensorySequence([]byte(strings.Join(alternate, "_")))
		}

		var scratch dmt.ClassificationScratch
		classification := tree.Classify(sequence, &scratch)
		predictions := tree.PredictNextSensoryTokens(parent, nil)
		done := make(chan struct{})

		go func() {
			_, _, _, _, _, _, _, _ = analyzer.cognitionVisualization(
				sequence, parent, parts[0], classification, predictions,
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
TestCognitionBranchesForkFromRoot proves Cortex exports one sensory root with
category transition forks beneath it, not a mixed symbol tree.
*/
func TestCognitionBranchesForkFromRoot(t *testing.T) {
	Convey("Given trained sequences that diverge after the sensory root", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		analyzer := &Analyzer{tree: tree}

		tree.TrainSensorySequence([]byte("s_pressure-positive_divergence-negative"))
		tree.TrainSensorySequence([]byte("s_pressure-negative_divergence-positive"))
		tree.TrainSensorySequence([]byte("s_pressure-positive_loaded-imbalance"))

		tip := tree.PredictNextSensoryTokens(
			[]byte("s_pressure-positive"),
			make([]dmt.LookaheadPrediction, 0, 4),
		)
		branches, count := analyzer.cognitionBranches(
			analyzer.treeExpandWidth(cognitionBeamWidth(tip)),
		)

		Convey("Then the root has one s child and category forks below it", func() {
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

			So(childrenOf[0], ShouldResemble, []string{"s"})

			sID := -1

			for _, branch := range branches {
				if branch.Token == "s" {
					sID = branch.ID
					break
				}
			}

			So(sID, ShouldBeGreaterThanOrEqualTo, 0)
			So(len(childrenOf[sID]), ShouldBeGreaterThanOrEqualTo, 2)
		})
	})
}

/*
TestCognitionBeamsStayGlobal proves MAP lookahead uses observed category
transitions under s and never scopes the prediction path by symbol.
*/
func TestCognitionBeamsStayGlobal(t *testing.T) {
	Convey("Given category continuations observed across symbols", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		analyzer := &Analyzer{tree: tree}

		for range 8 {
			tree.TrainSensorySequence([]byte("s_loaded-imbalance_median-depth"))
			tree.TrainSensorySequence([]byte("s_thermal-exhaustion_median-depth"))
		}

		sequence := []byte("s_loaded-imbalance_median-depth")

		var scratch dmt.ClassificationScratch
		classification := tree.Classify(sequence, &scratch)
		predictions := tree.PredictNextSensoryTokens([]byte("s"), nil)

		_, beams, _, _, _, _, _, _ := analyzer.cognitionVisualization(
			sequence, []byte("s_loaded-imbalance"), "s", classification, predictions,
		)

		Convey("Then the MAP beams stay under s without symbol prefixes", func() {
			So(len(beams), ShouldBeGreaterThan, 0)
			So(beams[0].Sequence, ShouldContainSubstring, "s_")
			So(beams[0].Sequence, ShouldNotContainSubstring, "usd")
		})
	})
}

func TestCognitionVisualization(t *testing.T) {
	Convey("Given a trained sensory sequence", t, func() {
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		sequence := []byte("s_pressure-positive")
		parts := []string{"s", "pressure-positive"}
		parent := []byte("s")
		analyzer := &Analyzer{tree: tree}

		tree.TrainSensorySequence(sequence)
		tree.TrainSensorySequence([]byte("s_pressure-negative"))
		tree.TrainSensorySequence([]byte("s_loaded-imbalance"))

		var scratch dmt.ClassificationScratch
		classification := tree.Classify(sequence, &scratch)
		predictions := tree.PredictNextSensoryTokens(parent, nil)

		branches, beams, classes, beamWidth, maxHops, nodeCount, lookaheadScore, lookaheadPaths :=
			analyzer.cognitionVisualization(
				sequence, parent, parts[0], classification, predictions,
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

	sequence := []byte("s_pressure-positive_divergence-negative")
	parts := []string{"s", "pressure-positive", "divergence-negative"}
	parent := []byte("s_pressure-positive")
	analyzer := &Analyzer{tree: tree}

	tree.TrainSensorySequence(sequence)
	tree.TrainSensorySequence([]byte("s_pressure-negative_divergence-positive"))
	tree.TrainSensorySequence([]byte("s_loaded-imbalance_divergence-positive"))

	var scratch dmt.ClassificationScratch
	classification := tree.Classify(sequence, &scratch)
	predictions := tree.PredictNextSensoryTokens(parent, nil)

	for b.Loop() {
		_, _, _, _, _, _, _, _ = analyzer.cognitionVisualization(
			sequence, parent, parts[0], classification, predictions,
		)
	}
}
