package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubscriptionSendLatest(t *testing.T) {
	Convey("Given a full subscription buffer", t, func() {
		subscription := &Subscription[int]{Channel: make(chan int, 1)}
		subscription.Send(1)

		Convey("It should replace stale data with the latest message", func() {
			subscription.SendLatest(2)

			So(<-subscription.Channel, ShouldEqual, 2)
		})
	})

	Convey("Given a full subscription buffer with several queued messages", t, func() {
		subscription := &Subscription[int]{Channel: make(chan int, 3)}
		subscription.Send(1)
		subscription.Send(2)
		subscription.Send(3)

		Convey("It should discard the oldest message and accept the latest message", func() {
			subscription.SendLatest(4)

			So(<-subscription.Channel, ShouldEqual, 2)
			So(<-subscription.Channel, ShouldEqual, 3)
			So(<-subscription.Channel, ShouldEqual, 4)
		})
	})
}
