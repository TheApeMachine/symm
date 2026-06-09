package types

import (
	"encoding/json"
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

			raw, ok := frame.Params.(json.RawMessage)

			So(ok, ShouldBeTrue)
			So(string(raw), ShouldContainSubstring, "ETH/EUR")
		})
	})
}

func TestNewKrakenMessageWithToken(t *testing.T) {
	Convey("Given authenticated params with a token field", t, func() {
		params := map[string]any{
			"channel": "level3",
			"token":   "venue-token",
		}

		frame, err := NewKrakenMessage("subscribe", params, 7)

		Convey("It should marshal the token onto the wire frame", func() {
			So(err, ShouldBeNil)

			raw, ok := frame.Params.(json.RawMessage)

			So(ok, ShouldBeTrue)
			So(string(raw), ShouldContainSubstring, `"token":"venue-token"`)
		})
	})
}
