package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	venue "github.com/theapemachine/symm/tests/venue"
)

func TestDesk(t *testing.T) {
	Convey("Given an initialized trading desk", t, func() {
		viper.Set("trading.allocation.max_fraction", 0.5)
		viper.Set("market.quote_currency", "USD")

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

		instrument := &Instrument{
			cache: &sync.Map{},
			quote: "USD",
		}
		price := NewPrice(ctx, api, instrument)
		price.fees.Store("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.25),
		})
		price.Update(&kraken.TickerData{
			Symbol: "BTC/USD",
			Ask:    decimal.NewFromFloat64(50000),
			Bid:    decimal.NewFromFloat64(49950),
		})

		balance := NewBalance(ctx, api)
		desk := NewDesk(ctx, api, price, balance)

		Convey("When entering a position for a symbol with valid cash", func() {
			pos := desk.Enter("BTC/USD")

			Convey("Then a position is created with calculated entry volume", func() {
				So(pos, ShouldNotBeNil)
				So(pos.EntryOrder, ShouldNotBeNil)
				So(pos.EntryOrder.Pair, ShouldEqual, "BTC/USD")
				So(pos.EntryOrder.Type, ShouldEqual, "buy")
				So(pos.EntryOrder.Volume, ShouldNotBeBlank)
				So(pos.ExitOrder.Pair, ShouldEqual, "BTC/USD")
				So(pos.ExitOrder.Type, ShouldEqual, "sell")
				So(pos.ExitOrder.Volume, ShouldEqual, pos.EntryOrder.Volume)
			})
		})

		Convey("When exiting an active position", func() {
			pos := desk.Enter("BTC/USD")
			So(pos, ShouldNotBeNil)

			err := desk.Exit(pos)
			So(err, ShouldBeNil)

			Convey("Then the exit response is recorded", func() {
				So(pos.ExitResponse, ShouldNotBeNil)
			})
		})

		Convey("When entering with an onPending callback", func() {
			var pendingPos *Position
			pos := desk.Enter("BTC/USD", func(pending *Position) {
				pendingPos = pending
			})

			So(pos, ShouldNotBeNil)
			So(pendingPos, ShouldNotBeNil)
			So(pendingPos.PositionID, ShouldEqual, pos.PositionID)
			So(pos.EntryOrder.ClOrdId, ShouldEqual, pos.PositionID)
		})

		Convey("When an entry position is partially filled", func() {
			pos := desk.Enter("BTC/USD")
			So(pos, ShouldNotBeNil)

			// Record partial fill of 0.001 volume
			pos.SetFill(decimal.NewFromFloat64(50000.0), decimal.NewFromFloat64(0.001), decimal.NewFromFloat64(0.05))
			So(pos.ExecutedVolume().Float64(), ShouldEqual, 0.001)

			err := desk.Exit(pos)
			So(err, ShouldBeNil)

			Convey("Then the exit order volume matches the executed volume, not requested volume", func() {
				So(pos.ExitOrder.Volume, ShouldEqual, "0.001")
			})
		})

		Convey("When market experiences a severe structural fault (>3 crossed book ticks)", func() {
			price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)
			price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)
			price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)
			price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)

			So(price.Anomalies().HasSevereFault("BTC/USD"), ShouldBeTrue)

			pos := desk.Enter("BTC/USD")

			Convey("Then Enter short circuits, returns nil, and transitions desk to error", func() {
				So(pos, ShouldBeNil)
				So(desk.Status(), ShouldEqual, runtime.ERROR)

				Convey("When market stabilizes with clean ticks", func() {
					price.Anomalies().RecordClean("BTC/USD")
					price.Anomalies().RecordClean("BTC/USD")
					price.Anomalies().RecordClean("BTC/USD")

					So(price.Anomalies().HasSevereFault("BTC/USD"), ShouldBeFalse)
					So(desk.Status(), ShouldEqual, runtime.READY)
					So(price.Status(), ShouldEqual, runtime.READY)
				})
			})
		})
	})
}
