package broker_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
entryDecision builds a sized entry carrying the stop geometry it was sized
under.

The plan is not decoration. The desk refuses an entry without one, because the
quantity travelling with a decision was solved against a particular risk
distance and attaching some other distance after the fact breaks the coupling
that makes a wide stop affordable. A bare decision here would only be testing a
path production no longer has.
*/
func entryDecision(market *tests.Market, symbol string) types.Decision {
	decision := types.Decision{
		ID:               uuid.NewString(),
		Action:           types.ActionEnter,
		Symbol:           symbol,
		ProposedQuantity: decimal.NewFromFloat64(0.25),
		ProposedNotional: decimal.NewFromInt64(25),
	}

	if pair, err := market.Desk.Instrument().Pair(symbol); err == nil {
		decision.Risk = market.Desk.Price().RiskPlan(pair)
	}

	return decision
}

func TestDeskExecute(t *testing.T) {
	Convey("Given a production-wired simulated market", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
			testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		}

		Convey("Execute should submit entries but reject strategy exits", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			decision := entryDecision(market, symbols[0].Pair)

			So(market.Desk.Execute([]types.Decision{decision}), ShouldBeNil)
			So(market.Desk.OpenPositions(), ShouldEqual, 1)

			positions := slices.Collect(market.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(positions[0].ID, ShouldEqual, decision.ID)
			So(positions[0].EntryOrder.ClOrdId, ShouldEqual, decision.ID)
			So(positions[0].Status, ShouldEqual, types.PENDING)

			fill := executionfixture.BuyFill()
			fill.ClientOrderID = decision.ID
			fill.Symbol = decision.Symbol
			fill.AvgPrice = fillPrice(market, decision.Symbol)
			fill.CumQty = decision.ProposedQuantity.String()
			market.Private.Publish("executions", executionfixture.Frame(fill))
			market.Tick()

			err := market.Desk.Execute([]types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionExit,
				Symbol: decision.Symbol,
			}})

			positions = slices.Collect(market.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "only a triggered stoploss may submit a sell")
			So(positions[0].Status, ShouldEqual, types.OPEN)
			So(positions[0].ExitOrderResult, ShouldBeNil)
		}))

		Convey("A repeated enter should reject the new position", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			decision := entryDecision(market, symbols[0].Pair)
			So(market.Desk.Execute([]types.Decision{decision}), ShouldBeNil)
			openPositions := market.Desk.OpenPositions()

			duplicate := entryDecision(market, decision.Symbol)
			err := market.Desk.Execute([]types.Decision{duplicate})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "symbol already has an active position")
			So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(market.Desk.Positions()), ShouldHaveLength, openPositions)
		}))

		Convey("A strategy exit should be rejected without inspecting inventory", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			openPositions := market.Desk.OpenPositions()
			err := market.Desk.Execute([]types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionExit,
				Symbol: symbols[0].Pair,
			}})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "only a triggered stoploss may submit a sell")
			So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(market.Desk.Positions()), ShouldBeEmpty)
		}))

		Convey("An AddOrder failure should release the attempted position", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			market.Private.FailAddOrder(errors.New("venue unavailable"))
			openPositions := market.Desk.OpenPositions()
			err := market.Desk.Execute([]types.Decision{entryDecision(market, symbols[0].Pair)})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "failed to place market order")
			So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(market.Desk.Positions()), ShouldBeEmpty)
		}))

		Convey("Concurrent entries should not exceed normal capacity", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			executionErrors := make(chan error, len(symbols))
			wait := sync.WaitGroup{}

			for _, symbol := range symbols {
				wait.Add(1)

				go func() {
					defer wait.Done()
					executionErrors <- market.Desk.Execute([]types.Decision{entryDecision(market, symbol.Pair)})
				}()
			}

			wait.Wait()
			close(executionErrors)
			rejections := 0

			for err := range executionErrors {
				if err != nil {
					rejections++
				}
			}

			So(rejections, ShouldEqual, 1)
			So(market.Desk.OpenPositions(), ShouldEqual, market.Desk.MaxPositions())
			So(slices.Collect(market.Desk.Positions()), ShouldHaveLength, market.Desk.MaxPositions())
		}))
	})
}

func BenchmarkDeskExecute(b *testing.B) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
	}

	market := tests.NewMarket(b.Context(), symbols)
	defer market.Close()

	market.Tick()
	decision := entryDecision(market, symbols[0].Pair)

	for b.Loop() {
		_ = market.Desk.Execute([]types.Decision{decision})
	}
}
