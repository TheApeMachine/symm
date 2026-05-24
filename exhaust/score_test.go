package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExitScoreLong(t *testing.T) {
	Convey("Given thinning bid depth and fading buy pressure", t, func() {
		history := symbolHistory{
			bidDepths:  []float64{100, 95, 90, 40, 35},
			spreads:    []float64{10, 10, 10, 10, 10},
			pressures:  []float64{0.8, 0.75, 0.7, 0.2},
			imbalances: []float64{0.5, 0.4, 0.3, -0.1},
		}

		urgency, reason := exitScoreLong(history)

		Convey("It should recommend an early exit", func() {
			So(urgency, ShouldBeGreaterThan, 0.35)
			So(reason, ShouldNotBeBlank)
		})
	})
}

func TestExitScoreShort(t *testing.T) {
	Convey("Given pressure flipping against a short", t, func() {
		history := symbolHistory{
			bidDepths:  []float64{80, 75, 70, 65},
			spreads:    []float64{12, 12, 12, 12},
			pressures:  []float64{-0.7, -0.65, 0.2},
			imbalances: []float64{-0.4, -0.35, 0.2},
		}

		urgency, reason := exitScoreShort(history)

		Convey("It should recommend an early cover", func() {
			So(urgency, ShouldBeGreaterThan, 0.2)
			So(reason, ShouldNotBeBlank)
		})
	})
}

func BenchmarkExitScoreLong(b *testing.B) {
	history := symbolHistory{
		bidDepths:  []float64{100, 95, 90, 40, 35},
		spreads:    []float64{10, 10, 10, 10, 10},
		pressures:  []float64{0.8, 0.75, 0.7, 0.2},
		imbalances: []float64{0.5, 0.4, 0.3, -0.1},
	}

	b.ReportAllocs()

	for b.Loop() {
		exitScoreLong(history)
	}
}
