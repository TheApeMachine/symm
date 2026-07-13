package broker

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
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
