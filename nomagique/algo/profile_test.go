package algo_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestLagProfileSupportAndUnits(t *testing.T) {
	Convey("The lag profile retains every candidate, including its support", t, func() {
		times := []int64{0, 1e9, 2e9, 3e9, 4e9, 5e9}
		left := prices(times, []float64{1, 2, 1.5, 3, 2.2, 4})
		right := prices([]int64{1e9, 2e9, 3e9, 4e9, 5e9, 6e9}, []float64{1, 2, 1.5, 3, 2.2, 4})
		node := correlation.NewLagProfile(algo.NewHayashiYoshida(), 1e9, 2.0)
		profile := tests.CollectSeq[correlation.LagCandidate](node.Next(transport.NewValues(correlation.LagProfileInput{Left: left, Right: right}).Next(nil)))
		So(node.Error(), ShouldBeNil)
		So(len(profile), ShouldEqual, 5)

		peakNode := correlation.NewPeak()
		points := make([]correlation.Point, 0, len(profile))

		for _, candidate := range profile {
			So(candidate.Support, ShouldBeGreaterThan, -1)
			points = append(points, correlation.Point{X: candidate.X, Y: candidate.Y})
		}

		peakOut := tests.CollectSeq[correlation.PeakResult](peakNode.Next(transport.NewValues(points...).Next(nil)))
		So(peakNode.Error(), ShouldBeNil)
		So(len(peakOut), ShouldEqual, 1)
		So(peakOut[0].Point.X, ShouldEqual, 1)
		So(peakOut[0].Point.Y, ShouldEqual, 1)
	})
}

func TestProfileCurvatureSeconds(t *testing.T) {
	Convey("Curvature and prominence use the neighbouring ordinates around the peak", t, func() {
		points := []correlation.Point{{-1, 0.1}, {0, 0.9}, {1, 0.3}}
		curvNode := correlation.NewCurvature()
		curvature := tests.CollectSeq[float64](curvNode.Next(transport.NewValues(points...).Next(nil)))
		So(curvNode.Error(), ShouldBeNil)
		So(len(curvature), ShouldEqual, 1)
		So(curvature[0], ShouldEqual, 1.4)

		promNode := correlation.NewProminence()
		prominence := tests.CollectSeq[float64](promNode.Next(transport.NewValues(points...).Next(nil)))
		So(promNode.Error(), ShouldBeNil)
		So(len(prominence), ShouldEqual, 1)
		So(prominence[0], ShouldEqual, 0.7)
	})
}
