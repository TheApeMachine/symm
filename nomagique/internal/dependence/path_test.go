package dependence

import (
	. "github.com/smartystreets/goconvey/convey"
	"math"
	"testing"
)

func TestPathCompare(t *testing.T) {
	Convey("Strict asynchronous overlaps agree with an independent interval-pair sum", t, func() {
		left := Path{Points: []Point{{0, 1}, {2e9, math.E}, {4e9, math.Exp(3)}}}
		right := Path{Points: []Point{{0, 1}, {1e9, math.E}, {3e9, math.Exp(2)}, {4e9, math.Exp(4)}}}
		left.Measure()
		right.Measure()
		for _, lag := range []int64{-5e9, -1e9, 0, 1e9, 5e9} {
			support, covariance := 0.0, 0.0
			for _, first := range left.Returns {
				for _, second := range right.Returns {
					if first.From+lag < second.To && second.From < first.To+lag {
						support++
						covariance += first.Value * second.Value
					}
				}
			}
			result := left.Compare(&right, lag)
			So(result.Support, ShouldEqual, support)
			So(result.Covariance, ShouldAlmostEqual, covariance)
			So(result.Defined, ShouldEqual, support > 0)
			if support > 0 {
				So(result.Correlation, ShouldAlmostEqual, covariance/math.Sqrt(left.Energy*right.Energy))
			}
		}
	})
}

func TestPathMeasure(t *testing.T) {
	Convey("Return energy is counted once and physical rates are per second", t, func() {
		path := Path{Points: []Point{{0, 1}, {1e9, math.E}, {3e9, math.Exp(3)}}}
		path.Measure()
		So(path.Energy, ShouldAlmostEqual, 5)
		So(path.Rate, ShouldAlmostEqual, 1.5)
		So(path.Spacing, ShouldEqual, 1.5e9)
		path.Measure()
		So(len(path.Returns), ShouldEqual, 2)
		So(path.Energy, ShouldAlmostEqual, 5)
	})
}
