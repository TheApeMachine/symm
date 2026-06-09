package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewKrakenMessage(t *testing.T) {
	Convey("Given add_order params", t, func() {
		frame, err := NewKrakenMessage("add_order", map[string]any{
			"symbol": "ETH/EUR",
			"side":   "buy",
		}, 42)

		Convey("It should marshal params onto the wire envelope", func() {
			So(err, ShouldBeNil)
			So(frame.Method, ShouldEqual, "add_order")
			So(frame.ReqID, ShouldEqual, 42)
			So(string(frame.Params), ShouldContainSubstring, "ETH/EUR")
		})
	})
}
