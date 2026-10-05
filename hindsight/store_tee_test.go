package hindsight_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestStoreTeeWaitFree(t *testing.T) {
	Convey("Given a wait-free StoreTee off-ramp", t, func() {
		ctx := context.Background()
		tee := hindsight.NewStoreTee(ctx, "wait-free")
		tee.Transition(runtime.READY)

		Convey("Push enqueues without blocking the pipeline", func() {
			for index := 0; index < 100; index++ {
				measurement := data.NewMeasurement("websocket", nil)
				measurement.Label = "BTC/USD"
				measurement.SeqIdx = int64(index + 1)
				tee.Push(data.Publication{Measurement: measurement})
			}

			So(tee.Pending(), ShouldEqual, 100)
			So(tee.Error(), ShouldBeNil)
		})

		Convey("Next drains publications in FIFO order", func() {
			for index := 0; index < 5; index++ {
				measurement := data.NewMeasurement("websocket", nil)
				measurement.Label = "BTC/USD"
				measurement.SeqIdx = int64(index + 1)
				tee.Push(data.Publication{Measurement: measurement})
			}

			for index := 0; index < 5; index++ {
				ptr := tee.Next()
				So(ptr, ShouldNotBeNil)
				pub := (*data.Publication)(ptr)
				So(pub.Measurement.SeqIdx, ShouldEqual, int64(index+1))
			}

			So(tee.Pending(), ShouldEqual, 0)
			So(tee.Next() == nil, ShouldBeTrue)
		})
	})
}
