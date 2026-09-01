package runtime

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewStatus(t *testing.T) {
	Convey("Given a fresh status", t, func() {
		status := NewStatus()

		Convey("It begins in init", func() {
			So(status.Current(), ShouldEqual, INIT)
		})
	})
}

func TestTransition(t *testing.T) {
	Convey("Given a status in init", t, func() {
		status := NewStatus()

		Convey("A legal transition moves forward", func() {
			status.Transition(READY)

			So(status.Current(), ShouldEqual, READY)
		})

		Convey("Re-entering the current stage is a no-op, not an error", func() {
			status.Transition(INIT)

			So(status.Current(), ShouldEqual, INIT)
			So(status.err, ShouldBeNil)
		})

		Convey("An illegal transition records an error", func() {
			status.Transition(DONE)

			So(status.Current(), ShouldEqual, INIT)
			So(status.err, ShouldNotBeNil)
		})
	})

	Convey("Given a status in error", t, func() {
		status := NewStatus()
		status.Transition(ERROR)

		Convey("Re-entering error is a no-op, not an error", func() {
			status.Transition(ERROR)

			So(status.Current(), ShouldEqual, ERROR)
			So(status.err, ShouldBeNil)
		})

		Convey("A legal recovery transitions away from error", func() {
			status.Transition(READY)

			So(status.Current(), ShouldEqual, READY)
		})
	})

	Convey("Given a connected transport awaiting subscription completion", t, func() {
		status := NewStatus()
		status.Transition(BUSY)

		Convey("subscription admission transitions it directly to ready", func() {
			status.Transition(READY)

			So(status.Current(), ShouldEqual, READY)
			So(status.err, ShouldBeNil)
		})
	})
}
