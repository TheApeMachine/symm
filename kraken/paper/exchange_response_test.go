package paper

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestWebSocketDefersExchangeResponses(t *testing.T) {
	Convey("Given a paper websocket balance delivery", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)

		trading.ResetDeskReady()
		t.Cleanup(trading.ResetDeskReady)

		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.quote_currency", "EUR")
		t.Cleanup(viper.Reset)

		balances := NewBalances(&WebSocket{
			ctx:         ctx,
			cancel:      cancel,
			pool:        pool,
			broadcasts:  make(map[string]*qpool.BroadcastGroup),
			subscribers: make(map[string]*qpool.Subscriber),
		}, NewIdentifier(), NewPairCatalog(ctx))

		Convey("It should mark the desk ready on balance snapshot delivery", func() {
			ws := balances.socket
			ws.deliverPrivateResponse(nil, balances.snapshot())

			So(trading.DeskReady(), ShouldBeTrue)
		})
	})
}
