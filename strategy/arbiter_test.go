package strategy_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func TestArbiterArbitrate(t *testing.T) {
	Convey("Given normal slots are occupied", t, tests.WithFixtureOrders(t, []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
	}, func(market *tests.Market) {
		for range 16 {
			market.Tick()
		}

		quantity := decimal.NewFromFloat64(0.25)

		for _, symbol := range market.Symbols[:market.Desk.MaxPositions()] {
			err := market.Desk.Execute([]types.Decision{{
				ID:               uuid.NewString(),
				Action:           types.ActionEnter,
				Symbol:           symbol.Pair,
				ProposedQuantity: quantity,
			}})

			So(err, ShouldBeNil)
		}

		So(market.Desk.OpenSlots(false), ShouldEqual, 0)
		So(market.Desk.OpenSlots(true), ShouldEqual, 2)

		thesis := types.NewThesis()
		thesis.Decisions = []types.Decision{{
			ID:                uuid.NewString(),
			Action:            types.ActionEnter,
			Symbol:            market.Symbols[2].Pair,
			At:                thesis.At,
			Utility:           1.0,
			OpportunityMargin: 1.0,
		}}

		arbiter := strategy.NewArbiter(market.Desk, market.Desk.Price())
		arbiter.Arbitrate(thesis)

		So(thesis.Decisions, ShouldHaveLength, 1)
		So(thesis.Decisions[0].Action, ShouldEqual, types.ActionNothing)
		So(thesis.Decisions[0].Cause, ShouldEqual, "slots_full")
		So(thesis.Decisions[0].AllocationClass, ShouldNotEqual, "reserved")
	}))

	Convey("Given all normal and reserved slots are occupied", t, tests.WithFixtureOrders(t, []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		testtypes.NewSymbol("SIM4/USD", 100.0, 5150),
		testtypes.NewSymbol("SIM5/USD", 100.0, 8080),
	}, func(market *tests.Market) {
		for range 16 {
			market.Tick()
		}

		quantity := decimal.NewFromFloat64(0.25)
		totalSlots := market.Desk.OpenSlots(true)

		for positionIndex, symbol := range market.Symbols[:totalSlots] {
			err := market.Desk.Execute([]types.Decision{{
				ID:               uuid.NewString(),
				Action:           types.ActionEnter,
				Symbol:           symbol.Pair,
				ProposedQuantity: quantity,
				Opportunity:      positionIndex >= market.Desk.MaxPositions(),
			}})

			So(err, ShouldBeNil)
		}

		So(market.Desk.OpenSlots(true), ShouldEqual, 0)

		thesis := types.NewThesis()
		thesis.Decisions = []types.Decision{{
			ID:                uuid.NewString(),
			Action:            types.ActionEnter,
			Symbol:            market.Symbols[4].Pair,
			At:                thesis.At,
			Utility:           1.0,
			Opportunity:       true,
			OpportunityMargin: 1.0,
		}}

		arbiter := strategy.NewArbiter(market.Desk, market.Desk.Price())
		arbiter.Arbitrate(thesis)

		So(thesis.Decisions, ShouldHaveLength, 1)
		So(thesis.Decisions[0].Action, ShouldEqual, types.ActionNothing)
		So(thesis.Decisions[0].Cause, ShouldEqual, "slots_full")
	}))
}
