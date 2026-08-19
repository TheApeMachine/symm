package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubscriptionSend(t *testing.T) {
	Convey("Given a bounded subscription with one buffered slot", t, func() {
		subscription := &Subscription[int]{Channel: make(chan int, 1)}

		Convey("It should enqueue the first message without blocking", func() {
			subscription.Send(1)

			So(<-subscription.Channel, ShouldEqual, 1)
		})

		Convey("It should drain the oldest message instead of blocking when full", func() {
			subscription.Send(1)
			subscription.Send(2)

			So(<-subscription.Channel, ShouldEqual, 2)
		})
	})
}
