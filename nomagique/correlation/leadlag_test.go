package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
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
		var original []equation.LagCandidate

		for range 2 {
			out, err := transport.Evaluate(node, transport.Values(equation.LagProfileInput{
				Left:  prices(times, left),
				Right: prices(times, right),
			}))
			So(err, ShouldBeNil)
			So(out.X, ShouldEqual, 2)
			So(out.Spacing, ShouldEqual, 1e9)
			So(out.Span, ShouldEqual, 14)
			So(out.LagIndex, ShouldEqual, 2)
			So(out.Defined, ShouldBeTrue)
			So(len(out.Profile), ShouldEqual, 29)
			So(out.Profile[int(out.Index)].Support, ShouldEqual, out.Support)
			So(out.Support, ShouldBeGreaterThan, 0)

			if original == nil {
				original = out.Profile
			}
		}

		empty, err := transport.Evaluate(node, transport.Values(equation.LagProfileInput{}))
		So(err, ShouldBeNil)
		So(len(empty.Profile), ShouldEqual, 0)
		So(empty.Defined, ShouldBeFalse)
		So(original[0].X, ShouldEqual, -14)
	})
}

func TestLagShapeUsesSelectedIndex(t *testing.T) {
	Convey("Shape uses the selected index, not a new peak search", t, func() {
		profile := make([]equation.LagCandidate, 5)

		for index, y := range []float64{.1, .2, 1, .8, .2} {
			profile[index] = equation.LagCandidate{
				LagEstimate: equation.LagEstimate{Support: 1, LeftEnergy: 1, RightEnergy: 1, Correlation: y},
				Index:       float64(index),
				X:           float64(index - 2),
				Y:           y,
			}
		}

		out, err := transport.Evaluate(correlation.NewLagShape(), transport.Values(correlation.LagShapeInput{
			Profile: profile,
			Index:   3,
			Span:    2,
			Spacing: 1e9,
		}))
		So(err, ShouldBeNil)
		So(out.Prominence, ShouldAlmostEqual, .2)
		So(out.Curvature, ShouldAlmostEqual, .4)
	})
}
