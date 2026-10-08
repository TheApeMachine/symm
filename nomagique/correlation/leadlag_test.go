package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestLeadLagNext(t *testing.T) {
	Convey("A two-second lag is recovered with its profile intact", t, func() {
		times := make([]int64, 16)
		left, right := make([]float64, 16), make([]float64, 16)

		for index := range times {
			times[index] = int64(index) * 1e9
			left[index] = math.Exp(math.Sin(float64(index)))
		}

		for index := range times {
			right[index] = left[max(0, index-2)]
		}

		node := correlation.NewLeadLag(algo.NewHayashiYoshida())
		var original []correlation.LagCandidate

		for range 2 {
			out := tests.CollectSeq[correlation.LeadLagReading](node.Next(transport.NewValues(correlation.LagProfileInput{
				Left:  prices(times, left),
				Right: prices(times, right),
			}).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			got := out[0]
			So(got.X, ShouldEqual, 2)
			So(got.Spacing, ShouldEqual, 1e9)
			So(got.Span, ShouldEqual, 14)
			So(got.LagIndex, ShouldEqual, 2)
			So(got.Defined, ShouldBeTrue)
			So(len(got.Profile), ShouldEqual, 29)
			So(got.Profile[int(got.Index)].Support, ShouldEqual, got.Support)
			So(got.Support, ShouldBeGreaterThan, 0)

			if original == nil {
				original = got.Profile
			}
		}

		empty := tests.CollectSeq[correlation.LeadLagReading](node.Next(transport.NewValues(correlation.LagProfileInput{}).Next(nil)))
		So(node.Error(), ShouldBeNil)
		So(len(empty[0].Profile), ShouldEqual, 0)
		So(empty[0].Defined, ShouldBeFalse)
		So(original[0].X, ShouldEqual, -14)
	})
}

func TestLagShapeUsesSelectedIndex(t *testing.T) {
	Convey("Shape uses the selected index, not a new peak search", t, func() {
		profile := make([]correlation.LagCandidate, 5)

		for index, y := range []float64{.1, .2, 1, .8, .2} {
			profile[index] = correlation.LagCandidate{
				LagEstimate: correlation.LagEstimate{Support: 1, LeftEnergy: 1, RightEnergy: 1, Correlation: y, Defined: true},
				Index:       float64(index),
				X:           float64(index - 2),
				Y:           y,
			}
		}

		out := tests.CollectSeq[correlation.LagShapeResult](correlation.NewLagShape().Next(transport.NewValues(correlation.LagShapeInput{
			Profile: profile,
			Index:   3,
			Span:    2,
			Spacing: 1e9,
		}).Next(nil)))
		So(len(out), ShouldEqual, 1)
		So(out[0].Prominence, ShouldAlmostEqual, .2)
		So(out[0].Curvature, ShouldAlmostEqual, .4)
	})
}
