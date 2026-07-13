package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestValuationUpdate(t *testing.T) {
	Convey("Given a filled long position and an executable ticker", t, func() {
		entryPrice, err := decimal.NewFromString("100")
		So(err, ShouldBeNil)
		feeRate, err := decimal.NewFromString("0.26")
		So(err, ShouldBeNil)
		bid, err := decimal.NewFromString("110")
		So(err, ShouldBeNil)
		ask, err := decimal.NewFromString("111")
		So(err, ShouldBeNil)

		position := &Position{Data: &PositionData{
			Symbol:     "BTC/USD",
			Qty:        *decimal.NewFromInt64(2),
			EntryPrice: *entryPrice,
			FeeRate:    feeRate,
		}}

		stop, triggered, err := (&Valuation{}).Update(position.Data, position.Stop, kraken.TickerData{
			Symbol: "BTC/USD",
			Bid:    bid,
			Ask:    ask,
		})
		position.Stop = stop

		Convey("It should publish the executable mark and fee-aware return", func() {
			So(err, ShouldBeNil)
			So(triggered, ShouldBeFalse)
			So(position.Data.Mark.Float64(), ShouldEqual, 110.0)
			So(position.Data.PnL.Float64(), ShouldAlmostEqual, 18.908, 1e-9)
			So(position.Data.ReturnPct, ShouldAlmostEqual, 9.454, 1e-9)
		})

		Convey("It should derive a trailing stop from observed dispersion", func() {
			So(position.Stop, ShouldNotBeNil)
			So(position.Stop.Symbol, ShouldEqual, "BTC/USD")
			So(position.Stop.PeakReturn, ShouldAlmostEqual, 0.10, 1e-9)
			So(position.Stop.StopPrice.Float64(), ShouldAlmostEqual, 108.428, 1e-9)
			So(position.Stop.StopReturn, ShouldAlmostEqual, 0.08428, 1e-9)
		})
	})

	Convey("Given an armed stop", t, func() {
		entryPrice, err := decimal.NewFromString("100")
		So(err, ShouldBeNil)
		feeRate, err := decimal.NewFromString("0.26")
		So(err, ShouldBeNil)
		peak, err := decimal.NewFromString("120")
		So(err, ShouldBeNil)
		stopPrice, err := decimal.NewFromString("118.8")
		So(err, ShouldBeNil)
		bid, err := decimal.NewFromString("118.7")
		So(err, ShouldBeNil)
		ask, err := decimal.NewFromString("118.8")
		So(err, ShouldBeNil)

		position := &Position{
			Data: &PositionData{
				Symbol:     "BTC/USD",
				Qty:        *decimal.NewFromInt64(1),
				EntryPrice: *entryPrice,
				FeeRate:    feeRate,
			},
			Stop: &StopData{
				Symbol:    "BTC/USD",
				Armed:     true,
				PeakPrice: *peak,
				StopPrice: *stopPrice,
			},
		}

		valuation := &Valuation{}
		valuation.previous = entryPrice
		stop, triggered, err := valuation.Update(position.Data, position.Stop, kraken.TickerData{
			Symbol: "BTC/USD",
			Bid:    bid,
			Ask:    ask,
		})
		position.Stop = stop

		Convey("It should trigger without loosening the existing stop", func() {
			So(err, ShouldBeNil)
			So(triggered, ShouldBeTrue)
			So(position.Stop.StopPrice.Float64(), ShouldEqual, 118.8)
		})
	})
}

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
			So(position.Data.FeeRate.Float64(), ShouldEqual, 0.26)
			So(len(ui), ShouldEqual, 1)
		})
	})
}

func BenchmarkPositionMark(b *testing.B) {
	entryPrice, _ := decimal.NewFromString("100")
	feeRate, _ := decimal.NewFromString("0.26")
	bid, _ := decimal.NewFromString("110")
	ask, _ := decimal.NewFromString("111")
	position := &Position{Data: &PositionData{
		Symbol:     "BTC/USD",
		Qty:        *decimal.NewFromInt64(2),
		EntryPrice: *entryPrice,
		FeeRate:    feeRate,
	}}
	ticker := kraken.TickerData{Symbol: "BTC/USD", Bid: bid, Ask: ask}

	b.ReportAllocs()

	for b.Loop() {
		position.Stop, _, _ = (&Valuation{}).Update(
			position.Data, position.Stop, ticker,
		)
	}
}
