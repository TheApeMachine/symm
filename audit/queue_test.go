package audit

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestWriterQueuePush(t *testing.T) {
	convey.Convey("Given an open writer queue", t, func() {
		queue := newWriterQueue()

		convey.Convey("It should return frames in enqueue order", func() {
			convey.So(queue.Push(map[string]any{"seq": 1}), convey.ShouldBeNil)
			convey.So(queue.Push(map[string]any{"seq": 2}), convey.ShouldBeNil)

			first, ok := queue.Pop()
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(first["seq"], convey.ShouldEqual, 1)

			second, ok := queue.Pop()
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(second["seq"], convey.ShouldEqual, 2)
		})
	})
}

func TestWriterQueueClose(t *testing.T) {
	convey.Convey("Given a closed writer queue", t, func() {
		queue := newWriterQueue()
		queue.Close()

		convey.Convey("It should reject new frames and let the worker stop", func() {
			convey.So(queue.Push(map[string]any{}), convey.ShouldNotBeNil)

			frame, ok := queue.Pop()
			convey.So(frame, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}
