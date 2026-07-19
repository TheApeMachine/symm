package broker

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestPositionExecutionAck verifies confirmed fills mark inventory through Price
friction and close the lot when a sell fills.
*/
func TestPositionExecutionAck(t *testing.T) {
	Convey("Given an existing strategy holding and a confirmed entry fill", t, func() {
		fees := &sync.Map{}
		fees.Store("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		tickers := &sync.Map{}
		tickers.Store("BTC/USD", &kraken.TickerData{
			Symbol: "BTC/USD",
			Ask:    decimal.NewFromInt64(100),
			Bid:    decimal.NewFromInt64(100),
			Last:   decimal.NewFromInt64(100),
		})
		price := &Price{
			fees:    fees,
			tickers: tickers,
		}
		price.status.Store(types.READY)
		holdings := &sync.Map{}
		holding := types.NewHolding(
			context.Background(),
			"BTC/USD",
			decimal.NewFromInt64(1),
		)
		holding.Asset = "BTC"
		holdings.Store("BTC/USD", holding)
		balance := &Balance{quote: "USD", holdings: holdings}
		request := kraken.NewMarketOrder("buy", 1, "BTC/USD")
		position := &Position{
			status: types.PENDING,
			price:  price, balance: balance,
			pair: &kraken.InstrumentPair{
				Symbol: "BTC/USD", Base: "BTC",
				QtyIncrement: 0.00000001, QtyMin: 0.00000001,
				CostPrecision: 8,
			},
			request: request,
		}
		position.OrderAck([]byte(`{"method":"add_order","result":{"order_id":"order-1"},"success":true,"req_id":` +
			strconv.FormatInt(request.ReqID, 10) + `}`))
		So(position.Status(), ShouldEqual, types.PENDING)
		execution := &kraken.Execution{
			Channel: "executions", Type: "update",
			Data: []kraken.ExecutionData{{
				OrderID: "order-1", ExecID: "buy-1", ExecType: "trade",
				Symbol: "BTC/USD", Side: "buy", OrderType: "market",
				LastQty: 1, CumQty: 1, OrderStatus: "filled",
				LastPrice:   decimal.NewFromInt64(100),
				AvgPrice:    decimal.NewFromInt64(100),
				Cost:        decimal.NewFromInt64(100),
				FeeUsdEquiv: decimal.NewFromFloat64(0.26), Timestamp: time.Unix(1, 0),
			}},
		}
		buffer, err := execution.MarshalJSON()

		So(err, ShouldBeNil)
		position.ExecutionAck(buffer)

		Convey("It should open the lot and record WithFriction flatten-now PnL", func() {
			So(position.Status(), ShouldEqual, types.OPEN)
			So(holding.Status, ShouldEqual, types.OPEN)
			So(holding.Qty.Float64(), ShouldEqual, 1.0)
			So(holding.Mark.Float64(), ShouldEqual, 100.0)
			So(holding.PnL, ShouldNotBeNil)
			// (bid 100 − exit 0.26) − (entry 100 + entry 0.26)
			So(holding.PnL.Float64(), ShouldAlmostEqual, -0.52, 1e-8)
			So(position.executions, ShouldHaveLength, 1)
		})

		Convey("It should keep remainder inventory after a partial sell", func() {
			holding.Qty = decimal.NewFromFloat64(0.0138685)
			exitRequest := kraken.NewMarketOrder("sell", 0.00884981, "BTC/USD")
			position.request = exitRequest
			position.OrderAck([]byte(`{"method":"add_order","result":{"order_id":"sell-partial"},"success":true,"req_id":` +
				strconv.FormatInt(exitRequest.ReqID, 10) + `}`))
			exit := &kraken.Execution{
				Channel: "executions", Type: "update",
				Data: []kraken.ExecutionData{{
					OrderID: "sell-partial", ExecID: "sell-partial-fill",
					ExecType: "trade", Symbol: "BTC/USD", Side: "sell",
					OrderType: "market", LastQty: 0.00884981, CumQty: 0.00884981,
					OrderStatus: "filled",
					LastPrice:   decimal.NewFromInt64(101),
					AvgPrice:    decimal.NewFromInt64(101),
					Cost:        decimal.NewFromInt64(16),
					FeeUsdEquiv: decimal.NewFromFloat64(0.04), Timestamp: time.Unix(2, 0),
				}},
			}
			exitBuffer, exitErr := exit.MarshalJSON()

			So(exitErr, ShouldBeNil)
			position.ExecutionAck(exitBuffer)
			live, holdingErr := balance.Holding("BTC/USD")

			So(holdingErr, ShouldBeNil)
			So(position.Status(), ShouldEqual, types.OPEN)
			So(live.Status, ShouldEqual, types.OPEN)
			So(live.Qty.Float64(), ShouldAlmostEqual, 0.00501869, 1e-7)
		})

		Convey("It should close and evict the holding when the position exits", func() {
			tickers.Store("BTC/USD", &kraken.TickerData{
				Symbol: "BTC/USD",
				Ask:    decimal.NewFromInt64(101),
				Bid:    decimal.NewFromInt64(101),
				Last:   decimal.NewFromInt64(101),
			})
			exitRequest := kraken.NewMarketOrder("sell", 1, "BTC/USD")
			position.request = exitRequest
			position.OrderAck([]byte(`{"method":"add_order","result":{"order_id":"sell-1"},"success":true,"req_id":` +
				strconv.FormatInt(exitRequest.ReqID, 10) + `}`))
			exit := &kraken.Execution{
				Channel: "executions", Type: "update",
				Data: []kraken.ExecutionData{{
					OrderID: "sell-1", ExecID: "sell-fill", ExecType: "trade",
					Symbol: "BTC/USD", Side: "sell", OrderType: "market",
					LastQty: 1, CumQty: 1, OrderStatus: "filled",
					LastPrice:   decimal.NewFromInt64(101),
					AvgPrice:    decimal.NewFromInt64(101),
					Cost:        decimal.NewFromInt64(101),
					FeeUsdEquiv: decimal.NewFromFloat64(0.2626), Timestamp: time.Unix(2, 0),
				}},
			}
			exitBuffer, exitErr := exit.MarshalJSON()

			So(exitErr, ShouldBeNil)
			position.ExecutionAck(exitBuffer)
			closed, holdingErr := balance.Holding("BTC/USD")

			So(holdingErr, ShouldBeNil)
			So(closed.Status, ShouldEqual, types.CLOSED)
			So(position.Status(), ShouldEqual, types.CLOSED)
			So(position.executions, ShouldHaveLength, 2)
		})

		Convey("It should keep the holding open when an exit is canceled", func() {
			exitRequest := kraken.NewMarketOrder("sell", 1, "BTC/USD")
			position.request = exitRequest
			position.OrderAck([]byte(`{"method":"add_order","result":{"order_id":"sell-1"},"success":true,"req_id":` +
				strconv.FormatInt(exitRequest.ReqID, 10) + `}`))
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
			So(position.Status(), ShouldEqual, types.OPEN)
		})
	})
}

/*
TestPositionExitQuantity verifies parameterless Exit sizes the sell from the
full filled holding balance before the transport accepts or rejects it.
*/
func TestPositionExitQuantity(t *testing.T) {
	Convey("Given an open position with one filled unit", t, func() {
		ctx := context.Background()
		mock := mockapi.NewMockAPI()
		paper := websocket.NewPaper(
			ctx, websocket.NewLatencySimulator(system.NewBooter(ctx, nil)),
		)
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), paper)
		holdings := &sync.Map{}
		holding := types.NewHolding(ctx, "BTC/USD", decimal.NewFromInt64(1))
		holding.Asset = "BTC"
		holding.Status = types.OPEN
		holdings.Store("BTC/USD", holding)
		balance := &Balance{
			quote:    "USD",
			holdings: holdings,
			status:   types.READY,
			model: &kraken.Balance{
				Type: "snapshot",
				Data: []kraken.BalanceData{{
					Asset: "BTC", Balance: decimal.NewFromInt64(1),
					Available: decimal.NewFromInt64(1),
				}},
			},
		}
		pair := &kraken.InstrumentPair{
			Symbol: "BTC/USD", Base: "BTC",
			QtyIncrement: 0.00000001, QtyMin: 0.00000001,
		}

		Convey("When Exit closes the position", func() {
			position := NewPosition(api, nil, nil, balance, pair)
			position.status = types.OPEN
			err := position.Exit()

			Convey("Then the sell order uses the full filled balance", func() {
				So(position.request, ShouldNotBeNil)
				So(position.request.Params.OrderQty, ShouldEqual, 1.0)

				if err != nil {
					So(err.Error(), ShouldContainSubstring, "failed to place market order")
				}
			})
		})
	})
}

/*
TestPositionSellMissingWalletFlattensGhost ensures a local OPEN shell without a
wallet Available row is closed instead of sent to the venue as a sell.
*/
func TestPositionSellMissingWalletFlattensGhost(t *testing.T) {
	Convey("Given an OPEN holding without a wallet Available row", t, func() {
		ctx := context.Background()
		holdings := &sync.Map{}
		holding := types.NewHolding(ctx, "ETH/USD", decimal.NewFromFloat64(0.0015))
		holding.Asset = "ETH"
		holding.Status = types.OPEN
		holdings.Store("ETH/USD", holding)
		balance := &Balance{
			quote:    "USD",
			holdings: holdings,
			model: &kraken.Balance{
				Type: "snapshot",
				Data: []kraken.BalanceData{{
					Asset: "USD", Balance: decimal.NewFromInt64(100),
					Available: decimal.NewFromInt64(100),
				}},
			},
			status: types.READY,
		}
		position := &Position{
			status:  types.OPEN,
			balance: balance,
			pair: &kraken.InstrumentPair{
				Symbol: "ETH/USD", Base: "ETH",
				QtyIncrement: 0.00000001, QtyMin: 0.00000001,
			},
		}

		err := position.Sell(nil)

		Convey("Then the ghost lot is flattened and no pending exit remains", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no wallet availability")
			So(holding.Status, ShouldEqual, types.CLOSED)
			So(holding.Qty.Sign(), ShouldEqual, 0)
			So(position.Pending(), ShouldBeFalse)
		})
	})
}

/*
TestPositionSellVenueRejectFlattensStaleAvailable ensures a venue rejection
closes the lot even when the local wallet snapshot still shows Available > 0.
*/
func TestPositionSellVenueRejectFlattensStaleAvailable(t *testing.T) {
	Convey("Given a stale wallet Available and a venue that rejects the sell", t, func() {
		ctx := context.Background()
		mock := mockapi.NewMockAPI()
		paper := websocket.NewPaper(
			ctx, websocket.NewLatencySimulator(system.NewBooter(ctx, nil)),
		)
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), paper)
		holdings := &sync.Map{}
		holding := types.NewHolding(ctx, "ETH/USD", decimal.NewFromFloat64(0.0015))
		holding.Asset = "ETH"
		holding.Status = types.OPEN
		holdings.Store("ETH/USD", holding)
		balance := &Balance{
			quote:    "USD",
			holdings: holdings,
			api:      api,
			model: &kraken.Balance{
				Type: "snapshot",
				Data: []kraken.BalanceData{
					{
						Asset: "USD", Balance: decimal.NewFromInt64(100),
						Available: decimal.NewFromInt64(100),
					},
					{
						Asset: "ETH", Balance: decimal.NewFromFloat64(0.0015),
						Available: decimal.NewFromFloat64(0.0015),
					},
				},
			},
			status: types.READY,
		}
		position := &Position{
			status:  types.OPEN,
			balance: balance,
			api:     api,
			pair: &kraken.InstrumentPair{
				Symbol: "ETH/USD", Base: "ETH",
				QtyIncrement: 0.00000001, QtyMin: 0.00000001,
			},
		}

		err := position.Sell(nil)

		Convey("Then the lot is closed so Decide cannot re-arm the sell", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "failed to place market order")
			So(holding.Status, ShouldEqual, types.CLOSED)
			So(holding.Qty.Sign(), ShouldEqual, 0)
			So(position.Pending(), ShouldBeFalse)
			So(position.Status(), ShouldEqual, types.ERROR)
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
BenchmarkPositionExecutionAck measures the fill-to-Holding mark path.
*/
func BenchmarkPositionExecutionAck(b *testing.B) {
	fees := &sync.Map{}
	fees.Store("BTC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	tickers := &sync.Map{}
	tickers.Store("BTC/USD", &kraken.TickerData{
		Symbol: "BTC/USD",
		Ask:    decimal.NewFromInt64(100),
		Bid:    decimal.NewFromInt64(100),
		Last:   decimal.NewFromInt64(100),
	})
	price := &Price{fees: fees, tickers: tickers}
	price.status.Store(types.READY)
	execution := &kraken.Execution{
		Channel: "executions", Type: "update",
		Data: []kraken.ExecutionData{{
			OrderID: "order-1", ExecID: "buy-1", ExecType: "trade",
			Symbol: "BTC/USD", Side: "buy", OrderStatus: "filled",
			LastQty: 1, LastPrice: decimal.NewFromInt64(100),
			Cost:        decimal.NewFromInt64(100),
			FeeUsdEquiv: decimal.NewFromFloat64(0.26), Timestamp: time.Unix(1, 0),
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
			pair: &kraken.InstrumentPair{
				Symbol: "BTC/USD", Base: "BTC", CostPrecision: 8,
			},
			orderID: "order-1",
		}
		holdings.Store("BTC/USD", types.NewHolding(
			context.Background(),
			"BTC/USD",
			decimal.NewFromInt64(1),
		))
		position.ExecutionAck(buffer)
	}
}
