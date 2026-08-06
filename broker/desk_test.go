package broker_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
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
func entryDecision(system *cmd.System, symbol string) types.Decision {
	decision := types.Decision{
		ID:               uuid.NewString(),
		Action:           types.ActionEnter,
		Symbol:           symbol,
		ProposedQuantity: decimal.NewFromFloat64(0.25),
		ProposedNotional: decimal.NewFromInt64(25),
	}

	if pair, err := system.Desk.Instrument().Pair(symbol); err == nil {
		decision.Risk = system.Desk.Price().RiskPlan(pair)
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

		Convey("Execute should submit entries but reject strategy exits", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.Tick()
			decision := entryDecision(system, symbols[0].Pair)

			So(system.Desk.Execute(decision), ShouldBeNil)
			So(system.Desk.OpenPositions(), ShouldEqual, 1)

			positions := slices.Collect(system.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(positions[0].ID, ShouldEqual, decision.ID)
			So(positions[0].EntryOrder.ClOrdId, ShouldEqual, decision.ID)
			So(positions[0].Status, ShouldEqual, types.PENDING)

			fill := executionfixture.BuyFill()
			fill.ClientOrderID = decision.ID
			fill.Symbol = decision.Symbol
			fill.AvgPrice = fillPrice(system, decision.Symbol)
			fill.CumQty = decision.ProposedQuantity.String()
			market.Private.Publish("executions", executionfixture.Frame(fill))
			market.Tick()

			err := system.Desk.Execute(types.Decision{
				ID:     uuid.NewString(),
				Action: types.ActionExit,
				Symbol: decision.Symbol,
			})

			positions = slices.Collect(system.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "only a triggered stoploss may submit a sell")
			So(positions[0].Status, ShouldEqual, types.OPEN)
			So(positions[0].ExitOrderResult, ShouldBeNil)
		}))

		Convey("A repeated enter should reject the new position", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.Tick()
			decision := entryDecision(system, symbols[0].Pair)
			So(system.Desk.Execute(decision), ShouldBeNil)
			openPositions := system.Desk.OpenPositions()

			duplicate := entryDecision(system, decision.Symbol)
			err := system.Desk.Execute(duplicate)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "symbol already has an active position")
			So(system.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(system.Desk.Positions()), ShouldHaveLength, openPositions)
		}))

		Convey("A strategy exit should be rejected without inspecting inventory", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			openPositions := system.Desk.OpenPositions()
			err := system.Desk.Execute(types.Decision{
				ID:     uuid.NewString(),
				Action: types.ActionExit,
				Symbol: symbols[0].Pair,
			})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "only a triggered stoploss may submit a sell")
			So(system.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(system.Desk.Positions()), ShouldBeEmpty)
		}))

		Convey("An AddOrder failure should release the attempted position", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.Tick()
			market.Private.FailAddOrder(errors.New("venue unavailable"))
			openPositions := system.Desk.OpenPositions()
			err := system.Desk.Execute(entryDecision(system, symbols[0].Pair))

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "failed to place market order")
			So(system.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(system.Desk.Positions()), ShouldBeEmpty)
		}))

		Convey("Concurrent entries should not exceed normal capacity", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.Tick()
			executionErrors := make(chan error, len(symbols))
			wait := sync.WaitGroup{}

			for _, symbol := range symbols {
				wait.Add(1)

				go func() {
					defer wait.Done()
					executionErrors <- system.Desk.Execute(entryDecision(system, symbol.Pair))
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
			So(system.Desk.OpenPositions(), ShouldEqual, system.Desk.MaxPositions())
			So(slices.Collect(system.Desk.Positions()), ShouldHaveLength, system.Desk.MaxPositions())
		}))
	})
}

func BenchmarkDeskExecute(b *testing.B) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
	}

	market := tests.NewMarket(b.Context(), symbols)
	defer market.Close()

	public, private := market.Feeds()
	system := cmd.Boot(b.Context(), types.NewThesis(nil), public, private, nil)

	defer system.Close()

	market.Tick()
	decision := entryDecision(system, symbols[0].Pair)

	for b.Loop() {
		_ = system.Desk.Execute(decision)
	}
}
