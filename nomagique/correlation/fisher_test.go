package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestFisherNext(t *testing.T) {
	Convey("Fisher-z reports definedness and Bonferroni-adjusted tails", t, func() {
		node := correlation.NewFisher()

		for _, test := range []struct {
			correlation, support float64
			defined              float64
		}{
			{0, 103, 1}, {.8, 103, 1}, {-.8, 103, 1}, {1, 103, 1}, {-1, 103, 1},
			{1.2, 103, 0}, {.8, 3, 0}, {.8, 0, 0}, {.5, 103, 1},
		} {
			sample := [3]float64{test.correlation, test.support, 20}
			out := tests.CollectSeq[[6]float64](node.Next(tests.SliceToSeq([][3]float64{sample})))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0][0], ShouldEqual, test.defined)

			if test.defined == 1 {
				expected := math.Erfc(math.Abs(math.Atanh(test.correlation)*math.Sqrt(test.support-3)) / math.Sqrt2)
				adjusted := math.Min(1, 20*expected)
				So(out[0][1], ShouldAlmostEqual, expected)
				So(out[0][4], ShouldAlmostEqual, adjusted)
			}
		}
	})
}
