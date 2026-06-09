package types

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSocketMessageRelease(t *testing.T) {
	Convey("Given a populated socket message", t, func() {
		message := NewSocketMessage()
		message.Channel = "book"
		message.Type = "update"
		message.Data = json.RawMessage(`{"symbol":"BTC/EUR"}`)

		message.Release()

		fresh := NewSocketMessage()

		Convey("It should return a zeroed message to the pool", func() {
			So(fresh.Channel, ShouldBeEmpty)
			So(fresh.Type, ShouldBeEmpty)
			So(fresh.Data, ShouldBeNil)
		})
	})
}

func TestSocketMessageReleaseOnce(t *testing.T) {
	Convey("Given a socket message released only once per frame", t, func() {
		message := NewSocketMessage()
		message.Channel = "heartbeat"
		message.Release()

		fresh := NewSocketMessage()

		Convey("It should not recycle stale channel labels", func() {
			So(fresh.Channel, ShouldBeEmpty)
		})
	})
}
