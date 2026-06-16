package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSocketMessageDecode(t *testing.T) {
	Convey("Given a Kraken websocket v2 status frame", t, func() {
		payload := []byte(`{"channel":"status","type":"update","data":[{"version":"2.0.10","system":"online","api_version":"v2","connection_id":15483510049470446170}]}`)

		message := Acquire()

		defer message.Release()

		Convey("It should decode the wire envelope", func() {
			So(message.Decode(payload), ShouldBeNil)
			So(message.Channel, ShouldEqual, "status")
			So(message.Type, ShouldEqual, "update")
			So(string(message.Data), ShouldContainSubstring, `"system":"online"`)
		})
	})
}
