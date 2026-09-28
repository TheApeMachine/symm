package strategy

import (
	"sync"
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

				wireFrame := trader.PositionsWire()
				So(wireFrame, ShouldNotBeNil)
				So(len(wireFrame.Rows), ShouldEqual, 1)
				So(wireFrame.Rows[0].Holding.Symbol, ShouldEqual, "BTC/USD")
				So(wireFrame.Rows[0].Holding.Status, ShouldEqual, "active")
				So(wireFrame.Rows[0].Holding.EntryPrice, ShouldNotBeEmpty)
				So(wireFrame.Rows[0].Holding.Mark, ShouldNotBeEmpty)

				recentTrades, err := trader.RecentTrades(10)
				So(err, ShouldBeNil)
				So(len(recentTrades), ShouldEqual, 1)

				decFrame := trader.DecisionsWire()
				So(decFrame, ShouldNotBeNil)
				So(len(decFrame.Decisions), ShouldBeGreaterThanOrEqualTo, 1)
				So(decFrame.Decisions[0].Symbol, ShouldEqual, "BTC/USD")
			})

			Convey("When ActionExit is received", func() {
				trader.OnAction("BTC/USD", ActionExit)

				Convey("Then the position is closed", func() {
					So(trader.Holding("BTC/USD"), ShouldBeFalse)
					So(trader.PositionCount(), ShouldEqual, 0)

					wireFrame := trader.PositionsWire()
					So(wireFrame, ShouldNotBeNil)
					So(len(wireFrame.Rows), ShouldEqual, 0)
				})
			})

			Convey("When execution arrives via ApplyExecution", func() {
				pos := trader.Position("BTC/USD")
				So(pos, ShouldNotBeNil)

				exec := &kraken.Execution{
					Channel: "executions",
					Type:    "update",
					Data: []kraken.ExecutionData{
						{
							Symbol:        "BTC/USD",
							OrderID:       pos.OrderID(),
							ClientOrderID: pos.PositionID,
							AvgPrice:      decimal.NewFromFloat64(49980.0),
							CumQty:        decimal.NewFromFloat64(0.0015),
							FeeUsdEquiv:   decimal.NewFromFloat64(0.18),
						},
					},
				}

				trader.ApplyExecution(exec)

				So(pos.ExecutedPrice().Float64(), ShouldEqual, 49980.0)
				So(pos.ExecutedVolume().Float64(), ShouldEqual, 0.0015)
				So(pos.Fee().Float64(), ShouldEqual, 0.18)

				wireFrame := trader.PositionsWire()
				So(wireFrame, ShouldNotBeNil)
				So(len(wireFrame.Rows), ShouldEqual, 1)
				So(wireFrame.Rows[0].Holding.EntryPrice, ShouldEqual, "49980")
				So(wireFrame.Rows[0].Holding.Qty, ShouldEqual, "0.0015")
			})
		})

		Convey("When multiple concurrent ActionEnter decisions arrive for the same symbol", func() {
			var wg sync.WaitGroup

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					trader.OnAction("BTC/USD", ActionEnter)
				}()
			}

			wg.Wait()

			Convey("Then exactly one position exists and duplicate orders are prevented", func() {
				So(trader.Holding("BTC/USD"), ShouldBeTrue)
				So(trader.PositionCount(), ShouldEqual, 1)
			})
		})
	})
}
