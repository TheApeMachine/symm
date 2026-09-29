package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker/position"
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
			pos, err := desk.Enter("BTC/USD")

			Convey("Then a position regulator is created with calculated entry volume", func() {
				So(err, ShouldBeNil)
				So(pos, ShouldNotBeNil)
				So(pos.Pending, ShouldNotBeNil)
				So(pos.Pending.Pair, ShouldEqual, "BTC/USD")
				So(pos.Pending.Type, ShouldEqual, "buy")
				So(pos.Pending.Volume, ShouldNotBeBlank)
				So(pos.Status(), ShouldEqual, "entry_pending")
			})
		})

		Convey("When attempting to exit before inventory fills", func() {
			pos, err := desk.Enter("BTC/USD")
			So(err, ShouldBeNil)
			So(pos, ShouldNotBeNil)

			exitErr := desk.Exit(pos)
			Convey("Then Exit is rejected because no volume is held yet", func() {
				So(exitErr, ShouldNotBeNil)
			})
		})

		Convey("When entering with an onPending callback", func() {
			var pendingPos *position.Regulator
			pos, err := desk.Enter("BTC/USD", func(pending *position.Regulator) {
				pendingPos = pending
			})

			So(err, ShouldBeNil)
			So(pos, ShouldNotBeNil)
			So(pendingPos, ShouldNotBeNil)
			So(pendingPos.PositionID, ShouldEqual, pos.PositionID)
			So(pos.Pending.ClOrdId, ShouldEqual, pos.PositionID)
		})

		Convey("When an entry position is partially filled", func() {
			pos, enterErr := desk.Enter("BTC/USD")
			So(enterErr, ShouldBeNil)
			So(pos, ShouldNotBeNil)

			// Record partial fill of 0.001 volume via Reconcile with remainder canceled
			report := kraken.ExecutionData{
				OrderID:       "ORD-DESK-1",
				ClientOrderID: pos.PositionID,
				Symbol:        "BTC/USD",
				OrderStatus:   "canceled",
				CumQty:        decimal.NewFromFloat64(0.001),
				CumCost:       decimal.NewFromFloat64(50.0),
				FeeUsdEquiv:   decimal.NewFromFloat64(0.05),
			}
			recErr := pos.Reconcile(report)
			So(recErr, ShouldBeNil)
			So(pos.Volume().Float64(), ShouldEqual, 0.001)

			err := desk.Exit(pos)
			So(err, ShouldBeNil)

			Convey("Then the exit order volume matches the executed volume, not requested volume", func() {
				So(pos.Pending.Type, ShouldEqual, "sell")
				exitVol, parseErr := decimal.NewFromString(pos.Pending.Volume)
				So(parseErr, ShouldBeNil)
				So(exitVol.Float64(), ShouldEqual, 0.001)
			})
		})

		Convey("When market experiences a severe structural fault (>3 crossed book ticks)", func() {
			price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)
			price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)
			price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)
			price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)

			So(price.Anomalies().HasSevereFault("BTC/USD"), ShouldBeTrue)

			pos, _ := desk.Enter("BTC/USD")

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

		Convey("When market health experiences critical failure", func() {
			for range 10 {
				price.Anomalies().Record("BTC/USD", AnomalyCrossedBook)
			}

			pos, _ := desk.Enter("BTC/USD")

			Convey("Then Enter short circuits and halts execution", func() {
				So(pos, ShouldBeNil)
			})
		})
	})
}
