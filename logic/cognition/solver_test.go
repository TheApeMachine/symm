package cognition

import (
	"math"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

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
