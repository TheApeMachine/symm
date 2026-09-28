package strategy

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	venue "github.com/theapemachine/symm/tests/venue"
)

func TestTrader(t *testing.T) {
	Convey("Given an initialized trader", t, func() {
		ctx := t.Context()
		conn := venue.NewConn()
		conn.BalanceResult = kraken.NewBalance([]byte(`{
			"channel": "balances",
			"type": "snapshot",
			"data": [{"asset": "USD", "balance": 200.0}]
		}`))

		api := websocket.NewAPI(ctx, conn, conn, &websocket.FuturesLive{})
		api.Normalizer().Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "BTC", Decimals: 8, DisplayDecimals: 8},
				"USD": {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTCUSD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})
		api.Transition(runtime.READY)

		instrument := broker.NewInstrument(api)
		price := broker.NewPrice(ctx, api, instrument)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.25),
		})
		price.Update(&kraken.TickerData{
			Symbol: "BTC/USD",
			Ask:    decimal.NewFromFloat64(50000),
			Bid:    decimal.NewFromFloat64(49950),
		})

		balance := broker.NewBalance(ctx, api)
		trader := NewTrader(ctx, api, price, balance)

		Convey("When ActionEnter is received", func() {
			trader.OnAction("BTC/USD", ActionEnter)

			Convey("Then the trader holds the new position", func() {
				So(trader.Holding("BTC/USD"), ShouldBeTrue)
				So(trader.PositionCount(), ShouldEqual, 1)
				So(trader.Position("BTC/USD"), ShouldNotBeNil)
			})

			Convey("When ActionExit is received", func() {
				trader.OnAction("BTC/USD", ActionExit)

				Convey("Then the position is closed", func() {
					So(trader.Holding("BTC/USD"), ShouldBeFalse)
					So(trader.PositionCount(), ShouldEqual, 0)
				})
			})
		})
	})
}
