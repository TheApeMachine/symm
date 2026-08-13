package trader

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewLane(t *testing.T) {
	Convey("Given an SPSC analytical lane configuration", t, func() {
		wake := make(chan struct{}, 1)

		Convey("It should require exact bounded topology values", func() {
			_, err := newLane[int](3, 0, wake)
			So(err, ShouldNotBeNil)

			_, err = newLane[int](4, -1, wake)
			So(err, ShouldNotBeNil)

			lane, err := newLane[int](4, 0, wake)
			So(err, ShouldBeNil)
			So(lane.telemetry().Capacity, ShouldEqual, 4)
		})
	})
}

func TestLanePush(t *testing.T) {
	Convey("Given one producer and one consumer", t, func() {
		wake := make(chan struct{}, 1)
		lane, err := newLane[int](4, 0, wake)
		So(err, ShouldBeNil)

		for value := range 4 {
			So(lane.Push(t.Context(), value), ShouldBeNil)
		}

		Convey("It should retain every value in producer order", func() {
			for expected := range 4 {
				actual, ok := lane.Pop()
				So(ok, ShouldBeTrue)
				So(actual, ShouldEqual, expected)
			}

			_, ok := lane.Pop()
			So(ok, ShouldBeFalse)
			So(lane.telemetry().HighWater, ShouldEqual, 4)
		})

		Convey("It should report cancellation instead of dropping on saturation", func() {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			So(lane.Push(ctx, 5), ShouldEqual, context.Canceled)
			So(lane.telemetry().Saturations, ShouldEqual, 1)
		})
	})
}
