package broker_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
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
		}))
	})
}
