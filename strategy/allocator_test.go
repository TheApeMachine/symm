package strategy_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func TestAllocatorAllocate(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
	}

	Convey("Given a market with positive cash balance", t, tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
		for range 16 {
			market.Tick()
		}

		allocator := strategy.NewAllocator(
			context.Background(),
			market.Desk.Balance(),
			market.Desk.Instrument(),
			market.Desk.Price(),
		)

		Convey("An entry decision should be sized and marked ready", func() {
			thesis := types.NewThesis()
			thesis.Decisions = []types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionEnter,
				Symbol: "SIM1/USD",
				At:     thesis.At,
			}}

			err := allocator.Allocate(thesis)
			So(err, ShouldBeNil)
			So(thesis.Readiness.Allocation, ShouldBeTrue)
			So(thesis.Decisions[0].Action, ShouldEqual, types.ActionEnter)
			So(thesis.Decisions[0].ProposedQuantity, ShouldNotBeNil)
			So(thesis.Decisions[0].ProposedQuantity.Sign(), ShouldBeGreaterThan, 0)
			So(thesis.Decisions[0].ProposedNotional, ShouldNotBeNil)
			So(thesis.Decisions[0].ProposedNotional.Sign(), ShouldBeGreaterThan, 0)
			So(thesis.Decisions[0].ReferencePrice, ShouldNotBeNil)
		})

		Convey("An unconfigured pair should be rejected in place", func() {
			thesis := types.NewThesis()
			thesis.Decisions = []types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionEnter,
				Symbol: "UNKNOWN/USD",
				At:     thesis.At,
			}}

			err := allocator.Allocate(thesis)
			So(err, ShouldBeNil)
			So(thesis.Decisions[0].Action, ShouldEqual, types.ActionNothing)
			So(thesis.Decisions[0].Reason, ShouldEqual, "instrument pair unavailable")
		})
	}))
}

func BenchmarkAllocatorAllocate(b *testing.B) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
	}

	market := tests.NewMarket(b.Context(), symbols)
	defer market.Close()

	for range 16 {
		market.Tick()
	}

	allocator := strategy.NewAllocator(
		context.Background(),
		market.Desk.Balance(),
		market.Desk.Instrument(),
		market.Desk.Price(),
	)

	thesis := types.NewThesis()
	thesis.Decisions = []types.Decision{{
		ID:     uuid.NewString(),
		Action: types.ActionEnter,
		Symbol: "SIM1/USD",
		At:     thesis.At,
	}}

	for b.Loop() {
		thesis.Decisions[0].Action = types.ActionEnter
		thesis.Decisions[0].ProposedQuantity = nil
		thesis.Decisions[0].ProposedNotional = nil
		_ = allocator.Allocate(thesis)
	}
}
