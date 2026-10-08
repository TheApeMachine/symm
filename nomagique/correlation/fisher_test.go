package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestFisherNext(t *testing.T) {
	Convey("Fisher-z reports definedness and Bonferroni-adjusted tails", t, func() {
		node := correlation.NewFisher()

		for _, test := range []struct {
			correlation, support float64
			defined              bool
		}{
			{0, 103, true}, {.8, 103, true}, {-.8, 103, true}, {1, 103, true}, {-1, 103, true},
			{1.2, 103, false}, {.8, 3, false}, {.8, 0, false}, {.5, 103, true},
		} {
			sample := correlation.FisherSample{
				Correlation: test.correlation,
				Support:     test.support,
				SearchCount: 20,
			}
			out := tests.CollectSeq[correlation.FisherReading](node.Next(transport.NewValues(sample).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0].Defined, ShouldEqual, test.defined)

			if test.defined {
				expected := math.Erfc(math.Abs(math.Atanh(test.correlation)*math.Sqrt(test.support-3)) / math.Sqrt2)
				adjusted := math.Min(1, 20*expected)
				So(out[0].PValue, ShouldAlmostEqual, expected)
				So(out[0].SearchAdjustedPValue, ShouldAlmostEqual, adjusted)
			}
		}
	})
}
