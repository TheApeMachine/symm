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
}
