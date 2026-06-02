package private

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCloneOutboundFrame(t *testing.T) {
	Convey("Given an outbound subscribe frame", t, func() {
		cloned, ok := cloneOutboundFrame(map[string]any{
			"method": "subscribe",
			"params": map[string]any{"channel": "executions"},
		})

		Convey("It should deep-clone for replay", func() {
			So(ok, ShouldBeTrue)

			frame := cloned.(map[string]any)
			So(frame["method"], ShouldEqual, "subscribe")
		})
	})

	Convey("Given an unmarshalable value", t, func() {
		_, ok := cloneOutboundFrame(make(chan int))

		Convey("It should reject the clone", func() {
			So(ok, ShouldBeFalse)
		})
	})
}
