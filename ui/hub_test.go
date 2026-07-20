package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestHub_FocusMessage proves browser focus commands reach the bound analysis
owner and malformed commands cannot silently disable full field publication.
*/
func TestHub_FocusMessage(t *testing.T) {
	Convey("Given a hub bound to an analysis focus owner", t, func() {
		hub := &Hub{}
		focused := ""
		hub.BindFocus(func(symbol string) {
			focused = symbol
		})

		Convey("When the browser selects a symbol", func() {
			err := hub.focusMessage([]byte(`{"focus":"BTC/USD"}`))

			Convey("Then the exact symbol reaches the analysis owner", func() {
				So(err, ShouldBeNil)
				So(focused, ShouldEqual, "BTC/USD")
			})
		})

		Convey("When the browser sends malformed JSON", func() {
			err := hub.focusMessage([]byte(`{"focus":`))

			Convey("Then the command is rejected without changing focus", func() {
				So(err, ShouldNotBeNil)
				So(focused, ShouldBeEmpty)
			})
		})

		Convey("When the browser omits the requested symbol", func() {
			err := hub.focusMessage([]byte(`{}`))

			Convey("Then the command is rejected without changing focus", func() {
				So(err, ShouldNotBeNil)
				So(focused, ShouldBeEmpty)
			})
		})
	})

	Convey("Given a hub without an analysis focus owner", t, func() {
		hub := &Hub{}

		Convey("When a valid symbol command arrives", func() {
			err := hub.focusMessage([]byte(`{"focus":"BTC/USD"}`))

			Convey("Then the missing production binding is explicit", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

/*
BenchmarkHub_FocusMessage measures the exact websocket control decode and
dispatch path used when the browser changes its requested field projection.
*/
func BenchmarkHub_FocusMessage(b *testing.B) {
	hub := &Hub{}
	hub.BindFocus(func(string) {})
	message := []byte(`{"focus":"BTC/USD"}`)

	b.ResetTimer()

	for range b.N {
		if err := hub.focusMessage(message); err != nil {
			b.Fatal(err)
		}
	}
}
