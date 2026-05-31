package market

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewCandleSubscription(t *testing.T) {
	Convey("Given replay is active", t, func() {
		restoreReplay := forceReplayActive(true)
		defer restoreReplay()

		stream := NewCandleSubscription(context.Background(), 1, "BTC/EUR")
		_, open := <-stream

		Convey("It should not open a live Kraken websocket", func() {
			So(open, ShouldBeFalse)
		})
	})
}
