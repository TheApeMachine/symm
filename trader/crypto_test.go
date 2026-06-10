package trader

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestNewCryptoRegistersFuturesChannel(t *testing.T) {
	convey.Convey("Given a crypto trader bus", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		crypto := NewCrypto(ctx, pool, krakenmarket.NewInstrumentRegistry())

		defer crypto.cancel()

		convey.Convey("It should accept kraken:futures publish", func() {
			err := crypto.bus.Send("kraken:futures", "book", map[string]string{"event": "subscribe"})
			convey.So(err, convey.ShouldBeNil)
		})
	})
}
