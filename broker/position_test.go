package broker_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func TestPositionOnExecution(t *testing.T) {
	Convey("Given a position entered through the production-wired desk", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100.0, 42),
		}

		Convey("Execution updates should follow private transport correlation", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			decision := entryDecision(symbols[0].Pair)
			So(market.Desk.Execute([]types.Decision{decision}), ShouldBeNil)

			positions := slices.Collect(market.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			position := positions[0]

			Convey("A correlated order-open event without a fill should remain pending", func() {
				opened := executionfixture.BuyFill()
				opened.ClientOrderID = decision.ID
				opened.Symbol = decision.Symbol
				opened.ExecType = "new"
				opened.OrderStatus = "open"
				opened.LastQty = ""
				opened.CumQty = ""
				market.Private.Publish("executions", executionfixture.Frame(opened))
				market.Tick()

				position = slices.Collect(market.Desk.Positions())[0]
				So(position.Status, ShouldEqual, types.PENDING)
				So(position.Holding.Status, ShouldEqual, types.PENDING)
				So(market.Desk.OpenPositions(), ShouldEqual, 1)
			})

			Convey("A filled event with another client order ID should be ignored", func() {
				originalQuantity := position.Holding.Qty.String()
				openPositions := market.Desk.OpenPositions()
				uncorrelated := executionfixture.BuyFill()
				uncorrelated.ClientOrderID = uuid.NewString()
				uncorrelated.Symbol = decision.Symbol
				uncorrelated.CumQty = "1.5"
				market.Private.Publish("executions", executionfixture.Frame(uncorrelated))
				market.Tick()

				position = slices.Collect(market.Desk.Positions())[0]
				So(position.Status, ShouldEqual, types.PENDING)
				So(position.Holding.Status, ShouldEqual, types.PENDING)
				So(position.Holding.Qty.String(), ShouldEqual, originalQuantity)
				So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			})

			Convey("Correlated buy and sell fills should open and close inventory", func() {
				buy := executionfixture.BuyFill()
				buy.ClientOrderID = decision.ID
				buy.Symbol = decision.Symbol
				buy.CumQty = decision.ProposedQuantity.String()
				market.Private.Publish("executions", executionfixture.Frame(buy))
				market.Tick()

				position = slices.Collect(market.Desk.Positions())[0]
				So(position.Status, ShouldEqual, types.OPEN)
				So(position.Holding.Status, ShouldEqual, types.OPEN)
				So(position.Holding.SellableQty.String(), ShouldEqual, "0.25")
				So(market.Desk.OpenPositions(), ShouldEqual, 1)

				exitID := uuid.NewString()
				So(market.Desk.Execute([]types.Decision{{
					ID:     exitID,
					Action: types.ActionExit,
					Symbol: decision.Symbol,
				}}), ShouldBeNil)

				sell := executionfixture.ExitFill()
				sell.ClientOrderID = exitID
				sell.Symbol = decision.Symbol
				sell.CumQty = decision.ProposedQuantity.String()
				market.Private.Publish("executions", executionfixture.Frame(sell))
				market.Tick()

				position = slices.Collect(market.Desk.Positions())[0]
				So(position.Status, ShouldEqual, types.CLOSED)
				So(position.Holding.Status, ShouldEqual, types.CLOSED)
				So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
				So(market.Desk.OpenPositions(), ShouldEqual, 0)
			})

			Convey("Split fills should accumulate each execution fee exactly once", func() {
				firstBuy := executionfixture.BuyFill()
				firstBuy.ClientOrderID = decision.ID
				firstBuy.Symbol = decision.Symbol
				firstBuy.ExecID = "entry-fill-one"
				firstBuy.OrderStatus = "partially_filled"
				firstBuy.LastQty = "0.10"
				firstBuy.CumQty = "0.10"
				firstBuy.AvgPrice = "100"
				firstBuy.FeeUsdEquiv = "0.026"
				market.Private.Publish("executions", executionfixture.Frame(firstBuy))
				market.Tick()

				secondBuy := firstBuy
				secondBuy.ExecID = "entry-fill-two"
				secondBuy.OrderStatus = "filled"
				secondBuy.LastQty = "0.15"
				secondBuy.CumQty = "0.25"
				secondBuy.FeeUsdEquiv = "0.039"
				market.Private.Publish("executions", executionfixture.Frame(secondBuy))
				market.Private.Publish("executions", executionfixture.Frame(secondBuy))
				market.Tick()

				position = slices.Collect(market.Desk.Positions())[0]
				expectedFee, err := decimal.NewFromString("0.065")
				So(err, ShouldBeNil)
				So(position.Holding.EntryFee.Cmp(expectedFee), ShouldEqual, 0)

				exitID := uuid.NewString()
				So(market.Desk.Execute([]types.Decision{{
					ID:     exitID,
					Action: types.ActionExit,
					Symbol: decision.Symbol,
				}}), ShouldBeNil)

				firstSell := executionfixture.ExitFill()
				firstSell.ClientOrderID = exitID
				firstSell.Symbol = decision.Symbol
				firstSell.ExecID = "exit-fill-one"
				firstSell.OrderStatus = "partially_filled"
				firstSell.FeeUsdEquiv = "0.030"
				market.Private.Publish("executions", executionfixture.Frame(firstSell))

				secondSell := firstSell
				secondSell.ExecID = "exit-fill-two"
				secondSell.OrderStatus = "filled"
				secondSell.FeeUsdEquiv = "0.035"
				market.Private.Publish("executions", executionfixture.Frame(secondSell))
				market.Private.Publish("executions", executionfixture.Frame(secondSell))
				market.Tick()

				position = slices.Collect(market.Desk.Positions())[0]
				So(position.Holding.ExitFee.Cmp(expectedFee), ShouldEqual, 0)
			})
		}))
	})
}

func TestPositionOnTicker(t *testing.T) {
	Convey("Given a triggered stoploss with an exit already pending", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100.0, 42),
		}

		Convey("Ticker updates should not submit the sell again", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			decision := entryDecision(symbols[0].Pair)
			So(market.Desk.Execute([]types.Decision{decision}), ShouldBeNil)

			buy := executionfixture.BuyFill()
			buy.ClientOrderID = decision.ID
			buy.Symbol = decision.Symbol
			buy.CumQty = decision.ProposedQuantity.String()
			market.Private.Publish("executions", executionfixture.Frame(buy))
			market.Tick()

			position := slices.Collect(market.Desk.Positions())[0]
			position.Holding.Stoploss.Status = types.TRIGGERED
			exitID := uuid.NewString()
			So(market.Desk.Execute([]types.Decision{{
				ID:     exitID,
				Action: types.ActionExit,
				Symbol: decision.Symbol,
			}}), ShouldBeNil)
			So(position.Status, ShouldEqual, types.PENDING)
			So(position.ExitOrderResult, ShouldNotBeNil)
			firstOrderID := position.ExitOrderResult.ID[0]

			market.Tick()

			So(position.ExitOrderResult.ID[0], ShouldEqual, firstOrderID)
		}))
	})
}
