package market

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewTradeSubscriptionReturns(t *testing.T) {
	Convey("Given a parent context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Convey("It should return a feed immediately", func() {
			feed := NewTradeSubscription(ctx, "BTC/EUR")

			So(feed.Client, ShouldNotBeNil)
			So(feed.Stream, ShouldNotBeNil)
		})
	})
}
