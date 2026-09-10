package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
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
			{1.2, 103, false}, {.8, 3, false}, {.8, 0, false}, {math.NaN(), 103, false}, {.5, 103, true},
		} {
			out, err := transport.Evaluate(node, transport.Values(correlation.FisherSample{
				Correlation: test.correlation,
				Support:     test.support,
				SearchCount: 20,
			}))
			So(err, ShouldBeNil)
			So(out.Defined, ShouldEqual, test.defined)
			expected := math.NaN()
			adjusted := math.NaN()

			if test.defined {
				expected = math.Erfc(math.Abs(math.Atanh(test.correlation)*math.Sqrt(test.support-3)) / math.Sqrt2)
				adjusted = math.Min(1, 20*expected)
			}

			if test.defined {
				So(out.PValue, ShouldAlmostEqual, expected)
				So(out.SearchAdjustedPValue, ShouldAlmostEqual, adjusted)
			}

			if !test.defined {
				So(math.IsNaN(out.PValue), ShouldBeTrue)
				So(math.IsNaN(out.SearchAdjustedPValue), ShouldBeTrue)
			}
		}
	})
}
