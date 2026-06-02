package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestAppendRingMeasurement(t *testing.T) {
	convey.Convey("Given a ring window", t, func() {
		window := make([]perspectives.Measurement, 0, StoryRingCapacity)

		convey.Convey("When more than StoryRingCapacity rows are appended", func() {
			for index := range StoryRingCapacity + 5 {
				window = AppendRingMeasurement(window, perspectives.Measurement{
					Symbol: "BTC/EUR",
					Last:   float64(index),
				})
			}

			convey.Convey("It should retain only the latest StoryRingCapacity rows", func() {
				convey.So(len(window), convey.ShouldEqual, StoryRingCapacity)
				convey.So(window[0].Last, convey.ShouldEqual, float64(5))
				convey.So(window[len(window)-1].Last, convey.ShouldEqual, float64(StoryRingCapacity+4))
			})
		})
	})
}

func TestRingSnapshot(t *testing.T) {
	convey.Convey("Given a mixed ring window", t, func() {
		window := []perspectives.Measurement{
			{Symbol: "BTC/EUR", Category: perspectives.CategoryDivergentMove},
			{Symbol: "ETH/EUR", Category: perspectives.CategorySystemicBeta},
			{Symbol: "", Source: perspectives.SourceFluid},
		}

		convey.Convey("When snapshotting BTC/EUR", func() {
			snapshots := RingSnapshot(window, "BTC/EUR")

			convey.Convey("It should include global rows and matching symbol rows only", func() {
				convey.So(len(snapshots), convey.ShouldEqual, 2)
				convey.So(snapshots[0].Symbol, convey.ShouldEqual, "BTC/EUR")
				convey.So(snapshots[1].Source, convey.ShouldEqual, perspectives.SourceFluid)
			})
		})
	})
}

func BenchmarkAppendRingMeasurement(b *testing.B) {
	window := make([]perspectives.Measurement, 0, StoryRingCapacity)
	row := perspectives.Measurement{Symbol: "BTC/EUR", Last: 1}

	for b.Loop() {
		window = AppendRingMeasurement(window, row)
	}
}
