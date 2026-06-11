package trader

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

func TestNewCryptoRegistersFuturesChannel(t *testing.T) {
	convey.Convey("Given a crypto trader bus", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		crypto := NewCrypto(ctx, pool)

		defer crypto.cancel()

		convey.Convey("It should accept kraken:futures publish", func() {
			err := crypto.bus.Send(internal.ChannelKrakenFutures, "book", map[string]string{"event": "subscribe"})
			convey.So(err, convey.ShouldBeNil)
		})
	})
}

func TestCryptoResetSubscriptions(t *testing.T) {
	convey.Convey("Given a crypto trader with active subscriptions", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		crypto := NewCrypto(ctx, pool)

		defer crypto.cancel()

		crypto.instrument.pairs.Store("BTC/USD", true)
		crypto.instrument.anchorSubscribed.Store(true)

		convey.Convey("It should clear pair and anchor state on reset", func() {
			crypto.reset()

			_, subscribed := crypto.instrument.pairs.Load("BTC/USD")
			convey.So(subscribed, convey.ShouldBeFalse)
			convey.So(crypto.instrument.anchorSubscribed.Load(), convey.ShouldBeFalse)
			convey.So(subscribed, convey.ShouldBeFalse)
		})
	})
}
