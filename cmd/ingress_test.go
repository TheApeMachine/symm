package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubscribeRejection(t *testing.T) {
	Convey("Given venue method acknowledgements", t, func() {
		Convey("a rejected subscribe is an error", func() {
			err := subscribeRejection([]byte(`{"method":"subscribe","success":false,"error":"Token(s) not found","result":{"channel":"level3","symbol":"BTC/USD"}}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Token(s) not found")
			So(err.Error(), ShouldContainSubstring, "level3")
		})

		Convey("an accepted subscribe is not", func() {
			So(subscribeRejection([]byte(`{"method":"subscribe","success":true,"result":{"channel":"trade","symbol":"BTC/USD"}}`)), ShouldBeNil)
		})

		Convey("a rejected order ack is left to the order path", func() {
			So(subscribeRejection([]byte(`{"method":"add_order","success":false,"error":"EOrder:Insufficient funds"}`)), ShouldBeNil)
		})

		Convey("a subscribe ack without success is not treated as a rejection", func() {
			So(subscribeRejection([]byte(`{"method":"subscribe"}`)), ShouldBeNil)
		})

		Convey("non-JSON and pong frames are ignored", func() {
			So(subscribeRejection([]byte(`not json`)), ShouldBeNil)
			So(subscribeRejection([]byte(`{"method":"pong"}`)), ShouldBeNil)
		})
	})
}
