package logic

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBoundaryClampsFrame(testingTB *testing.T) {
	Convey("Given normal signal measurements", testingTB, func() {
		boundaries := newBoundaryClamps()
		at := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
		measurements := map[types.SourceType]*types.Measurement{
			types.SourceHawkes: hawkesMeasurement("BTC/USD", at, 0),
			types.SourceCVD:    cvdMeasurement("BTC/USD", at, 0),
		}

		Convey("When the boundary frame is built", func() {
			frame, err := boundaries.Frame("BTC/USD", measurements)

			Convey("Then measurements should become deposits and oscillators", func() {
				So(err, ShouldBeNil)
				So(frame.clamps, ShouldHaveLength, 2)
				So(frame.oscillators, ShouldHaveLength, 2)
				So(frame.netMomentum(), ShouldBeGreaterThan, 0)
				So(frame.netPressure(), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When the boundary is intervened", func() {
			frame, err := boundaries.Frame("BTC/USD", measurements)
			intervened := frame.Intervene()

			Convey("Then do() should preserve aligned momentum", func() {
				So(err, ShouldBeNil)
				So(intervened.clamps, ShouldHaveLength, len(frame.clamps))
				So(intervened.netMomentum(), ShouldEqual, frame.netMomentum())
			})
		})

		Convey("When one clamp opposes the observed direction", func() {
			measurements[types.SourceHawkes].Categories = []types.Category{
				{
					Type:       types.CategoryFrenzy,
					Confidence: 1,
					Strength:   1,
				},
			}
			measurements[types.SourceCVD].Categories = []types.Category{
				{
					Type:       types.CategoryVolumeStarvation,
					Confidence: 0.25,
					Strength:   1,
				},
			}
			frame, err := boundaries.Frame("BTC/USD", measurements)
			intervened := frame.Intervene()

			Convey("Then do() should remove only the opposing momentum", func() {
				So(err, ShouldBeNil)
				So(frame.netMomentum(), ShouldBeGreaterThan, 0)
				So(intervened.netMomentum(), ShouldBeGreaterThan, frame.netMomentum())
			})
		})

		Convey("When raw metric scale changes without category evidence changing", func() {
			measurements = map[types.SourceType]*types.Measurement{
				types.SourceFluid: fluidMeasurement("BTC/USD", at, 0),
			}
			measurements[types.SourceFluid].Metrics["reynolds"] = 1000000
			measurements[types.SourceFluid].Metrics["viscosity"] = 999999
			frame, err := boundaries.Frame("BTC/USD", measurements)

			Convey("Then the clamp should remain on the normalized category scale", func() {
				So(err, ShouldBeNil)
				So(frame.clamps, ShouldHaveLength, 1)
				So(frame.clamps[0].momX, ShouldBeGreaterThan, 0)
				So(frame.clamps[0].momX, ShouldBeLessThan, 1)
			})
		})

		Convey("When a required metric is missing", func() {
			measurements[types.SourceHawkes].Metrics = map[string]float64{
				"branchingRatio": 0.4,
			}
			frame, err := boundaries.Frame("BTC/USD", measurements)

			Convey("Then the boundary should reject only that clamp", func() {
				So(err, ShouldBeNil)
				So(frame.clamps, ShouldHaveLength, 1)
				So(frame.clamps[0].source, ShouldEqual, types.SourceCVD)
			})
		})

		Convey("When every measurement is unusable", func() {
			delete(measurements, types.SourceCVD)
			measurements[types.SourceHawkes].Metrics = map[string]float64{
				"branchingRatio": 0.4,
			}
			_, err := boundaries.Frame("BTC/USD", measurements)

			Convey("Then the boundary should report no usable clamps", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "no metric clamps")
				So(err.Error(), ShouldContainSubstring, "intensityRatio")
			})
		})
	})
}
