package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestPositionExecutionAckValuesHoldingImmediately verifies confirmed fills become
fee-inclusive published Holding state before another ticker is required.
*/
func TestPositionExecutionAckValuesHoldingImmediately(t *testing.T) {
	Convey("Given a confirmed entry fill and the current execution price", t, func() {
		fees := &sync.Map{}
		fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
		price := &Price{
			status:  types.READY,
			fees:    fees,
			tickers: &sync.Map{},
		}
		holdings := &sync.Map{}
		holdings.Store("BTC/USD", types.Holding{
			Symbol: "BTC/USD",
			Asset:  "BTC",
			Qty:    *decimal.NewFromInt64(0),
			Order: &spot.Order{
				Description: &spot.OrderDescription{Pair: "BTC/USD"},
				Volume:      decimal.NewFromInt64(1),
			},
		})
		balance := &Balance{quote: "USD", holdings: holdings}
		position := &Position{
			status: types.PENDING,
			price:  price, balance: balance,
			Stop: &StopData{Symbol: "BTC/USD"},
		}
		execution := &kraken.Execution{
			Channel: "executions", Type: "update",
			Data: []kraken.ExecutionData{{
				ExecID: "buy-1", ExecType: "trade", Symbol: "BTC/USD", Side: "buy",
				LastQty: 1, LastPrice: *decimal.NewFromInt64(100),
				Cost:        *decimal.NewFromInt64(100),
				FeeUsdEquiv: *decimal.NewFromFloat64(0.26), Timestamp: time.Unix(1, 0),
			}},
		}
		buffer, err := execution.MarshalJSON()

		So(err, ShouldBeNil)
		position.ExecutionAck(buffer)

		Convey("It should publish an open holding at the round-trip fee loss", func() {
			holding, holdingErr := balance.Holding("BTC/USD")

			So(holdingErr, ShouldBeNil)
			So(position.Status(), ShouldEqual, types.OPEN)
			So(holding.Qty.Float64(), ShouldEqual, 1.0)
			So(holding.EntryPrice.Float64(), ShouldEqual, 100.0)
			So(holding.Mark.Float64(), ShouldEqual, 100.0)
			So(holding.EntryFee.Float64(), ShouldAlmostEqual, 0.26, 0.0000001)
			So(holding.ExitFee.Float64(), ShouldAlmostEqual, 0.26, 0.0000001)
			So(holding.PnL.Float64(), ShouldAlmostEqual, -0.52, 0.0000001)
			So(holding.ReturnPct, ShouldAlmostEqual, -0.52, 0.0000001)
			So(holding.EntryAt, ShouldEqual, time.Unix(1, 0))
			So(holding.Executions, ShouldHaveLength, 1)
		})

		Convey("It should remove the filled quantity when the position exits", func() {
			exit := &kraken.Execution{
				Channel: "executions", Type: "update",
				Data: []kraken.ExecutionData{{
					ExecID: "sell-1", ExecType: "trade", Symbol: "BTC/USD", Side: "sell",
					LastQty: 1, LastPrice: *decimal.NewFromInt64(101),
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
			So(holding.Qty.Sign(), ShouldEqual, 0)
			So(holding.Mark.Float64(), ShouldEqual, 101.0)
			So(holding.PnL.Float64(), ShouldAlmostEqual, 0.4774, 0.0000001)
			So(holding.Executions, ShouldHaveLength, 2)
		})
	})
}

func TestPositionHydrateReconcilesClosedRoundTrip(t *testing.T) {
	Convey("Given a wallet lot after a closed round trip", t, func() {
		currentQuantity, err := decimal.NewFromString("1.0000000000004")

		So(err, ShouldBeNil)

		holdings := &sync.Map{}
		holdings.Store("BTC/USD", types.Holding{
			Asset: "BTC",
			Qty:   *currentQuantity,
			Order: &spot.Order{Description: &spot.OrderDescription{Pair: "BTC/USD"}},
		})

		balance := &Balance{quote: "USD", holdings: holdings}
		position := &Position{
			balance: balance,
			price:   &Price{},
			Stop:    &StopData{Symbol: "BTC/USD"},
		}

		position.Hydrate("BTC/USD", tests.TradesHistory(tests.TradesClosedRoundTripCurrentLot))

		Convey("It should open with reconciled executions", func() {
			So(position.Status(), ShouldEqual, types.OPEN)

			holding, holdingErr := balance.Holding("BTC/USD")

			So(holdingErr, ShouldBeNil)
			So(len(holding.Executions), ShouldEqual, 1)
			So(holding.Order.Volume.Cmp(currentQuantity), ShouldEqual, 0)
			So(holding.EntryPrice.Float64(), ShouldEqual, 120.0)
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

func BenchmarkPositionReconcile(b *testing.B) {
	currentQuantity, err := decimal.NewFromString("1.0000000000004")

	if err != nil {
		b.Fatal(err)
	}

	holding := types.Holding{
		Asset: "BTC",
		Qty:   *currentQuantity,
		Order: &spot.Order{Description: &spot.OrderDescription{Pair: "BTC/USD"}},
	}
	balance := &Balance{quote: "USD", holdings: &sync.Map{}}
	position := &Position{balance: balance}
	history := tests.TradesHistory(tests.TradesClosedRoundTripCurrentLot)

	b.ReportAllocs()

	for b.Loop() {
		_ = position.reconcile(history, "BTC/USD", holding)
	}
}

/*
BenchmarkPositionExecutionAck measures the fill-to-Holding accounting path.
*/
func BenchmarkPositionExecutionAck(b *testing.B) {
	fees := &sync.Map{}
	fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
	price := &Price{status: types.READY, fees: fees, tickers: &sync.Map{}}
	holdings := &sync.Map{}
	balance := &Balance{quote: "USD", holdings: holdings}
	position := &Position{
		status: types.PENDING,
		price:  price, balance: balance,
		Stop: &StopData{Symbol: "BTC/USD"},
	}
	execution := &kraken.Execution{
		Channel: "executions", Type: "update",
		Data: []kraken.ExecutionData{{
			ExecID: "buy-1", ExecType: "trade", Symbol: "BTC/USD", Side: "buy",
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
		holdings.Store("BTC/USD", types.Holding{
			Symbol: "BTC/USD", Asset: "BTC",
			Qty: *decimal.NewFromInt64(0),
			Order: &spot.Order{
				Description: &spot.OrderDescription{Pair: "BTC/USD"},
				Volume:      decimal.NewFromInt64(1),
			},
		})
		position.ExecutionAck(buffer)
	}
}
