package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLevel3FramesAdd(t *testing.T) {
	Convey("Given producer sequence two physically arrives before sequence one", t, func() {
		frames := NewLevel3Frames()
		second := level3Frame{sequence: 2, stream: channelLevel3, raw: []byte("second")}
		first := level3Frame{sequence: 1, stream: channelTrade, raw: []byte("first")}
		So(frames.Add(second), ShouldBeNil)

		_, ready := frames.Next()
		So(ready, ShouldBeFalse)

		Convey("When the missing earlier sequence arrives", func() {
			So(frames.Add(first), ShouldBeNil)
			observedFirst, firstReady := frames.Next()
			observedSecond, secondReady := frames.Next()

			Convey("It should restore the claimed observation order", func() {
				So(firstReady, ShouldBeTrue)
				So(secondReady, ShouldBeTrue)
				So(observedFirst.raw, ShouldResemble, []byte("first"))
				So(observedSecond.raw, ShouldResemble, []byte("second"))
			})
		})
	})
}
