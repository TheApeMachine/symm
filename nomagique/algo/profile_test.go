package algo_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestLagProfileSupportAndUnits(t *testing.T) {
	Convey("The lag profile retains every candidate, including its support", t, func() {
		times := []int64{0, 1e9, 2e9, 3e9, 4e9, 5e9}
		left := prices(times, []float64{1, 2, 1.5, 3, 2.2, 4})
		right := prices([]int64{1e9, 2e9, 3e9, 4e9, 5e9, 6e9}, []float64{1, 2, 1.5, 3, 2.2, 4})
		node := equation.NewLagProfile(algo.NewHayashiYoshida(), 1e9, 2.0)
		profile := tests.CollectSeq(node.Next(transport.Values(equation.LagProfileInput{Left: left, Right: right})))
		So(node.Error(), ShouldBeNil)
		So(len(profile), ShouldEqual, 5)

		points := make([]equation.Point, 0, len(profile))

		for _, candidate := range profile {
			So(candidate.Support, ShouldBeGreaterThan, -1)
			points = append(points, equation.Point{X: candidate.X, Y: candidate.Y})
		}

		peak, err := transport.Evaluate(equation.NewPeak(), transport.Values(points...))
		So(err, ShouldBeNil)
		So(peak.Point.X, ShouldEqual, 1)
		So(peak.Point.Y, ShouldEqual, 1)
	})
}

func TestProfileCurvatureSeconds(t *testing.T) {
	Convey("Curvature and prominence use the neighbouring ordinates around the peak", t, func() {
		points := []equation.Point{{-1, 0.1}, {0, 0.9}, {1, 0.3}}
		curvature, err := transport.Evaluate(equation.NewCurvature(), transport.Values(points...))
		So(err, ShouldBeNil)
		So(curvature, ShouldEqual, 1.4)

		prominence, err := transport.Evaluate(equation.NewProminence(), transport.Values(points...))
		So(err, ShouldBeNil)
		So(prominence, ShouldEqual, 0.7)
	})
}
