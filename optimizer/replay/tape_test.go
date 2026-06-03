package replay

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestPrecompileTapeRingWindow(t *testing.T) {
	convey.Convey("Given more than StoryRingCapacity measurements", t, func() {
		rows := make([]perspectives.Measurement, 0, StoryRingCapacity+10)

		for index := range StoryRingCapacity + 10 {
			rows = append(rows, perspectives.Measurement{
				Symbol:   "BTC/EUR",
				Source:   perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      float64(index),
				Last:     100 + float64(index),
			})
		}

		tape := PrecompileTape(rows)
		lastTick := tape.Ticks[len(rows)-1]

		convey.Convey("It should cap the decision snapshot to the story ring size", func() {
			convey.So(len(lastTick.Snapshots), convey.ShouldEqual, StoryRingCapacity)
			convey.So(lastTick.Snapshots[0].SNR, convey.ShouldEqual, 10)
		})
	})
}
