package runtime

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSink(t *testing.T) {
	Convey("Given a Sink with room for two values", t, func() {
		sink := NewSink[int](2)

		Convey("Step hands the value to the consumer and returns it unchanged", func() {
			So(sink.Step(7), ShouldEqual, 7)
			So(<-sink.Out(), ShouldEqual, 7)
			So(sink.Dropped(), ShouldEqual, 0)
		})

		Convey("A consumer that never reads is dropped past, never waited on", func() {
			sink.Step(1)
			sink.Step(2)

			// The ring must not block here. A third value with nobody reading
			// is the case that would deadlock the pipeline if the send were
			// allowed to wait.
			So(sink.Step(3), ShouldEqual, 3)
			So(sink.Dropped(), ShouldEqual, 1)

			// The values already accepted are still intact and in order: a
			// drop discards the newest offer, never corrupts the buffer.
			So(<-sink.Out(), ShouldEqual, 1)
			So(<-sink.Out(), ShouldEqual, 2)
		})

		Convey("Offering to a full sink allocates nothing", func() {
			sink.Step(1)
			sink.Step(2)

			allocations := testing.AllocsPerRun(100, func() {
				sink.Step(3)
			})

			So(allocations, ShouldEqual, 0)
		})
	})

	Convey("Given a Sink asked for no capacity", t, func() {
		sink := NewSink[int](0)

		Convey("It still accepts one value rather than dropping everything", func() {
			So(sink.Step(1), ShouldEqual, 1)
			So(sink.Dropped(), ShouldEqual, 0)
			So(<-sink.Out(), ShouldEqual, 1)
		})
	})
}
