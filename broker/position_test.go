package broker

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestPositionExecutionAck verifies confirmed fills update the existing strategy
holding without deriving wallet quantity or speculative round-trip valuation.
*/
func TestPositionExecutionAck(t *testing.T) {
	Convey("Given an existing strategy holding and a confirmed entry fill", t, func() {
		fees := &sync.Map{}
		fees.Store("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		price := &Price{
			fees:    fees,
			tickers: &sync.Map{},
		}
		price.status.Store(types.READY)
		holdings := &sync.Map{}
		holding := &types.Holding{
			Symbol: "BTC/USD",
			Asset:  "BTC",
			Qty:    decimal.NewFromInt64(1),
		}
		holdings.Store("BTC/USD", holding)
		balance := &Balance{quote: "USD", holdings: holdings}
		request := kraken.NewMarketOrder("buy", 1, "BTC/USD")
		position := &Position{
			status: types.PENDING,
			price:  price, balance: balance,
			pair: &kraken.InstrumentPair{
				Symbol: "BTC/USD", Base: "BTC", QtyIncrement: 0.00000001, QtyMin: 0.00000001,
			},
			request: request, Stop: &StopData{Symbol: "BTC/USD"},
		}
		position.OrderAck([]byte(`{"method":"add_order","result":{"order_id":"order-1"},"success":true,"req_id":` +
			strconv.Itoa(request.ReqID) + `}`))
		So(position.Status(), ShouldEqual, types.PENDING)
		execution := &kraken.Execution{
			Channel: "executions", Type: "update",
			Data: []kraken.ExecutionData{{
				OrderID: "order-1", ExecID: "buy-1", ExecType: "trade",
				Symbol: "BTC/USD", Side: "buy", OrderType: "market",
				LastQty: 1, CumQty: 1, OrderStatus: "filled",
				LastPrice:   *decimal.NewFromInt64(100),
				AvgPrice:    *decimal.NewFromInt64(100),
				Cost:        *decimal.NewFromInt64(100),
				FeeUsdEquiv: *decimal.NewFromFloat64(0.26), Timestamp: time.Unix(1, 0),
			}},
		}
		buffer, err := execution.MarshalJSON()

		So(err, ShouldBeNil)
		position.ExecutionAck(buffer)

		Convey("It should record the execution facts on that holding", func() {
			So(position.Status(), ShouldEqual, types.OPEN)
			So(holding.Qty.Float64(), ShouldEqual, 1.0)
			So(holding.EntryPrice.Float64(), ShouldEqual, 100.0)
			So(holding.Mark.Float64(), ShouldEqual, 100.0)
			So(holding.EntryFee.Float64(), ShouldAlmostEqual, 0.26, 0.0000001)
			So(holding.ExitFee, ShouldBeNil)
			So(*holding.EntryAt, ShouldEqual, time.Unix(1, 0))
			So(holding.Executions, ShouldHaveLength, 1)
		})

		Convey("It should remove the filled quantity when the position exits", func() {
			exitRequest := kraken.NewMarketOrder("sell", 1, "BTC/USD")
			position.request = exitRequest
			position.OrderAck([]byte(`{"method":"add_order","result":{"order_id":"sell-1"},"success":true,"req_id":` +
				strconv.Itoa(exitRequest.ReqID) + `}`))
			exit := &kraken.Execution{
				Channel: "executions", Type: "update",
				Data: []kraken.ExecutionData{{
					OrderID: "sell-1", ExecID: "sell-fill", ExecType: "trade",
					Symbol: "BTC/USD", Side: "sell", OrderType: "market",
					LastQty: 1, CumQty: 1, OrderStatus: "filled",
					LastPrice:   *decimal.NewFromInt64(101),
					AvgPrice:    *decimal.NewFromInt64(101),
					Cost:        *decimal.NewFromInt64(101),
					FeeUsdEquiv: *decimal.NewFromFloat64(0.2626), Timestamp: time.Unix(2, 0),
				}},
			}
			exitBuffer, exitErr := exit.MarshalJSON()

			So(exitErr, ShouldBeNil)
			position.ExecutionAck(exitBuffer)
			holding, holdingErr := balance.Holding("BTC/USD")

			So(holdingErr, ShouldBeNil)
			So(position.Status(), ShouldEqual, types.CLOSED)
			So(holding.Mark.Float64(), ShouldEqual, 101.0)
			So(holding.Executions, ShouldHaveLength, 2)
		})

		Convey("It should keep the holding open when an exit is canceled", func() {
			exitRequest := kraken.NewMarketOrder("sell", 1, "BTC/USD")
			position.request = exitRequest
			position.OrderAck([]byte(`{"method":"add_order","result":{"order_id":"sell-1"},"success":true,"req_id":` +
				strconv.Itoa(exitRequest.ReqID) + `}`))
			canceled := &kraken.Execution{
				Channel: "executions", Type: "update",
				Data: []kraken.ExecutionData{{
					OrderID: "sell-1", ExecType: "canceled", OrderStatus: "canceled",
					Symbol: "BTC/USD", Side: "sell", OrderType: "market",
				}},
			}
			canceledBuffer, canceledErr := canceled.MarshalJSON()

			So(canceledErr, ShouldBeNil)
			position.ExecutionAck(canceledBuffer)
			So(position.Status(), ShouldEqual, types.CANCELED)
		})
	})
}

func TestBalanceTradeMatchesSymbol(t *testing.T) {
	Convey("Given Kraken REST pair encodings", t, func() {
		balance := &Balance{quote: "USD"}

		Convey("It should match slash, compact, and asset-only forms", func() {
			So(balance.TradeMatchesSymbol("NPCUSD", "NPC/USD"), ShouldBeTrue)
			So(balance.TradeMatchesSymbol("NPC", "NPC/USD"), ShouldBeTrue)
			So(balance.TradeMatchesSymbol("NPC/USD", "NPC/USD"), ShouldBeTrue)
			So(balance.TradeMatchesSymbol("BTCUSD", "NPC/USD"), ShouldBeFalse)
		})
	})
}

/*
BenchmarkPositionExecutionAck measures the fill-to-Holding accounting path.
*/
func BenchmarkPositionExecutionAck(b *testing.B) {
	fees := &sync.Map{}
	fees.Store("BTC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price := &Price{fees: fees, tickers: &sync.Map{}}
	price.status.Store(types.READY)
	execution := &kraken.Execution{
		Channel: "executions", Type: "update",
		Data: []kraken.ExecutionData{{
			OrderID: "order-1", ExecID: "buy-1", ExecType: "trade",
			Symbol: "BTC/USD", Side: "buy",
			LastQty: 1, LastPrice: *decimal.NewFromInt64(100),
			Cost:        *decimal.NewFromInt64(100),
			FeeUsdEquiv: *decimal.NewFromFloat64(0.26), Timestamp: time.Unix(1, 0),
		}},
	}
	buffer, err := execution.MarshalJSON()

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		holdings := &sync.Map{}
		balance := &Balance{quote: "USD", holdings: holdings}
		position := &Position{
			status: types.PENDING,
			price:  price, balance: balance,
			pair:    &kraken.InstrumentPair{Symbol: "BTC/USD", Base: "BTC"},
			orderID: "order-1", Stop: &StopData{Symbol: "BTC/USD"},
		}
		holdings.Store("BTC/USD", &types.Holding{
			Symbol: "BTC/USD", Asset: "BTC",
			Qty: decimal.NewFromInt64(0),
		})
		position.ExecutionAck(buffer)
	}
}
