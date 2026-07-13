package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

func TestPositionOrderAck(t *testing.T) {
	Convey("Given a position waiting for one request identity", t, func() {
		ui := make(chan []byte, 8)
		position := &Position{
			reqID: 7,
			ui:    ui,
			Data:  &PositionData{Symbol: "BTC/USD"},
		}

		position.OrderAck([]byte(`{
			"method":"add_order",
			"result":{"order_id":"wrong"},
			"success":true,
			"req_id":8
		}`))

		Convey("It should ignore another position's acknowledgement", func() {
			So(position.orderID, ShouldBeEmpty)
			So(len(ui), ShouldEqual, 0)
		})

		position.OrderAck([]byte(`{
			"method":"add_order",
			"result":{"order_id":"right"},
			"success":true,
			"req_id":7
		}`))

		Convey("It should accept and publish its own acknowledgement", func() {
			So(position.orderID, ShouldEqual, "right")
			So(position.Status(), ShouldEqual, types.OPEN)
			So(len(ui), ShouldEqual, 1)
		})
	})
}

func TestPositionExecutionAck(t *testing.T) {
	Convey("Given a position receiving cumulative buy fills", t, func() {
		ui := make(chan []byte, 8)
		price := &Price{
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.status = types.READY
		price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.26"})
		position := &Position{
			orderID: "order-1",
			ui:      ui,
			price:   price,
			Data: &PositionData{
				Symbol: "BTC/USD",
			},
		}

		position.ExecutionAck([]byte(`{
			"channel":"executions",
			"type":"update",
			"data":[{
				"order_id":"order-1",
				"exec_id":"fill-2",
				"exec_type":"trade",
				"symbol":"BTC/USD",
				"side":"buy",
				"last_qty":1,
				"last_price":"110",
				"cost":"110",
				"order_status":"filled",
				"cum_qty":2,
				"cum_cost":"210",
				"avg_price":"105"
			}]
		}`))

		Convey("It should use exchange cumulative quantity and average price", func() {
			So(position.Data.Qty.Float64(), ShouldEqual, 2.0)
			So(position.Data.EntryPrice.Float64(), ShouldEqual, 105.0)
			So(position.Data.Mark.Float64(), ShouldEqual, 110.0)
			So(position.Status(), ShouldEqual, types.FILLED)
			So(position.Executions(), ShouldHaveLength, 1)
			So(len(ui), ShouldEqual, 1)
		})
	})
}

func TestPositionHydrate(t *testing.T) {
	Convey("Given a wallet holding several assets and trade history for two of them", t, func() {
		ui := make(chan []byte, 8)
		price := &Price{
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.status = types.READY
		price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
		price.tickers.Store("BTC/USD", &kraken.TickerData{
			Symbol: "BTC/USD",
			Last:   decimal.NewFromFloat64(63039.400),
		})

		balance := &Balance{
			quote: "USD",
			model: &kraken.Balance{
				Data: []kraken.BalanceData{
					// GALA sorts before BTC here on purpose: Hydrate must
					// not pair whichever holding it reaches first with a
					// different symbol's trade.
					{Asset: "GALA", Balance: *decimal.NewFromFloat64(13536.853376037476)},
					{Asset: "BTC", Balance: *decimal.NewFromFloat64(0.0001)},
				},
			},
		}

		history := &kraken.TradesHistory{
			Result: kraken.TradesHistoryResult{
				Trades: map[string]spot.Trade{
					"gala-buy": {
						Pair:   "GALAUSD",
						Type:   "buy",
						Price:  decimal.NewFromFloat64(0.00232),
						Volume: decimal.NewFromFloat64(13536.853376037476),
					},
					"btc-buy": {
						Pair:   "BTCUSD",
						Type:   "buy",
						Price:  decimal.NewFromFloat64(64129.900),
						Volume: decimal.NewFromFloat64(0.0001),
					},
				},
			},
		}

		position := &Position{
			api:     &websocket.API{},
			ui:      ui,
			price:   price,
			balance: balance,
			Data:    &PositionData{},
		}

		position.Hydrate("BTC/USD", history)

		Convey("It should hydrate the quantity from BTC's own holding, not GALA's", func() {
			So(position.Data.Qty.Float64(), ShouldEqual, 0.0001)
			So(position.Data.EntryPrice.Float64(), ShouldEqual, 64129.900)
			So(position.Status(), ShouldEqual, types.OPEN)
		})

		Convey("It should value the position off BTC's own 0.0001 quantity, not GALA's", func() {
			So(position.Data.PnL.Float64(), ShouldAlmostEqual, -0.142114, 0.000001)
		})
	})
}
