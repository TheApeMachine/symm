package cognition

import (
	"context"
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

		thesis.Categories.Store("BTC/USD", []types.Category{{
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
	thesis.Categories.Store("BTC/USD", []types.Category{{
		Symbol:     "BTC/USD",
		Type:       category,
		Confidence: 1,
		Strength:   1,
	}})
	thesis.Stamp(types.SourceCategories)

	return thesis
}

func BenchmarkUpdate(b *testing.B) {
	tree, err := dmt.NewTree("")

	if err != nil {
		b.Fatal(err)
	}

	solver := NewSolver(tree, nil, nil)
	thesis := cognitionThesis(types.CategoryVerticalIgnition)
	b.ReportAllocs()

	for b.Loop() {
		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
