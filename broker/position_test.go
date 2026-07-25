package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	orderackfixture "github.com/theapemachine/symm/tests/fixtures/orderack"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/config"
)

/*
TestExecutionAck proves a paper-shaped fill (compact pair, capital Side) clears
PENDING so the lot is open for Exit instead of stuck behind a phantom pending.
*/
func TestExecutionAck(t *testing.T) {
	Convey("Given an entered lot waiting on its paper fill", t, func() {
		simulator := websocket.NewSimulator()
		So(simulator.Initialize(), ShouldBeNil)

		paper := websocket.NewPaper(context.Background(), simulator, config.Fixture())
		api := websocket.NewAPI(context.Background(), paper, paper, paper)
		price := NewPrice(api)
		So(price.RememberFee("NEAR/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)
		balance := NewBalance(api, nil, make(chan []byte, 4), config.Fixture().Market)
		balance.status.Store(types.READY)

		pair := kraken.InstrumentPair{
			Symbol: "NEAR/USD",
			Base:   "NEAR",
			Quote:  "USD",
		}

		position := NewPosition(api, nil, price, balance, pair)
		holding := types.NewHolding(
			context.Background(), "NEAR/USD", decimal.NewFromFloat64(20.90318866),
		)
		holding.Asset = "NEAR"
		balance.StoreHolding(holding)

		position.status = types.PENDING
		position.orderID = "PAPER-00005"

		Convey("When a fill arrives before OrderAck binds the venue id", func() {
			position.orderID = ""
			position.request = kraken.NewMarketOrder(
				"buy", holding.Qty, holding.Symbol,
			)

			early := kraken.NewExecutionFromMap(datura.Map[any]{
				"id": "PAPER-00004", "order_id": "PAPER-00005",
				"pair": "NEARUSD", "side": "Buy", "volume": 20.90318866,
				"price": 1.8956, "cost": 39.624084423896, "fee": 0.1030226195021296,
				"status": "filled", "time": "2026-07-23T10:56:20Z",
			})
			earlyRaw, earlyErr := early.MarshalJSON()
			So(earlyErr, ShouldBeNil)
			position.ExecutionAck(earlyRaw)

			Convey("Then the in-flight request still opens the lot", func() {
				So(position.orderID, ShouldEqual, "PAPER-00005")
				So(position.Status(), ShouldEqual, types.OPEN)
				So(holding.Status, ShouldEqual, types.OPEN)

				Convey("And a late OrderAck does not demote the open lot", func() {
					position.OrderAckRaw(orderackfixture.Frame(orderackfixture.Options{
						ReqID:   position.request.ReqID,
						OrderID: "PAPER-00005",
						Success: true,
					}))
					So(position.Status(), ShouldEqual, types.OPEN)
				})
			})
		})

		Convey("When the venue reports the compact-pair Buy fill", func() {
			execution := kraken.NewExecutionFromMap(datura.Map[any]{
				"id": "PAPER-00006", "order_id": "PAPER-00005",
				"pair": "NEARUSD", "side": "Buy", "volume": 20.90318866,
				"price": 1.8956, "cost": 39.624084423896, "fee": 0.1030226195021296,
				"status": "filled", "time": "2026-07-23T10:56:21Z",
			})
			raw, err := execution.MarshalJSON()
			So(err, ShouldBeNil)

			position.ExecutionAck(raw)

			Convey("Then PENDING clears and entry economics land", func() {
				So(position.Status(), ShouldEqual, types.OPEN)
				So(holding.Status, ShouldEqual, types.OPEN)
				So(holding.EntryPrice.Float64(), ShouldAlmostEqual, 1.8956, 1e-9)
			})

			Convey("When the venue reports the matching Sell fill", func() {
				position.orderID = "PAPER-00007"
				position.status = types.PENDING

				sell := kraken.NewExecutionFromMap(datura.Map[any]{
					"id": "PAPER-00008", "order_id": "PAPER-00007",
					"pair": "NEARUSD", "side": "Sell", "volume": 20.90318866,
					"price": 1.8912, "cost": 39.532110393792, "fee": 0.1027834870238592,
					"status": "filled", "time": "2026-07-23T10:57:23Z",
				})
				sellRaw, sellErr := sell.MarshalJSON()
				So(sellErr, ShouldBeNil)

				position.ExecutionAck(sellRaw)

				Convey("Then the lot closes instead of remaining pending", func() {
					So(position.Status(), ShouldEqual, types.CLOSED)
					So(holding.Status, ShouldEqual, types.CLOSED)
					So(holding.ExitPrice, ShouldNotBeNil)
				})
			})
		})
	})
}

/*
BenchmarkExecutionAck measures paper fill acknowledgment cost.
*/
func BenchmarkExecutionAck(b *testing.B) {
	simulator := websocket.NewSimulator()
	_ = simulator.Initialize()
	paper := websocket.NewPaper(context.Background(), simulator, config.Fixture())
	api := websocket.NewAPI(context.Background(), paper, paper, paper)
	price := NewPrice(api)
	_ = price.RememberFee("NEAR/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	balance := NewBalance(api, nil, make(chan []byte, 1), config.Fixture().Market)
	balance.status.Store(types.READY)
	pair := kraken.InstrumentPair{Symbol: "NEAR/USD", Base: "NEAR", Quote: "USD"}
	position := NewPosition(api, nil, price, balance, pair)
	holding := types.NewHolding(
		context.Background(), "NEAR/USD", decimal.NewFromFloat64(1),
	)
	balance.StoreHolding(holding)
	position.orderID = "PAPER-00005"
	raw, _ := kraken.NewExecutionFromMap(datura.Map[any]{
		"id":       "PAPER-00006",
		"order_id": "PAPER-00005",
		"pair":     "NEARUSD",
		"side":     "Buy",
		"volume":   1.0,
		"price":    1.9,
		"cost":     1.9,
		"fee":      0.01,
		"status":   "filled",
	}).MarshalJSON()

	for b.Loop() {
		position.status = types.PENDING
		holding.Status = types.PENDING
		holding.EntryPrice = nil
		holding.Qty = decimal.NewFromFloat64(1)
		position.ExecutionAck(raw)
	}
}
