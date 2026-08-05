package strategy_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
			market.Desk,
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

		Convey("A sized entry should carry the geometry it was sized under", func() {
			thesis := types.NewThesis()
			thesis.Decisions = []types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionEnter,
				Symbol: "SIM1/USD",
				At:     thesis.At,
			}}

			So(allocator.Allocate(thesis), ShouldBeNil)

			plan := thesis.Decisions[0].Risk
			So(plan.Present, ShouldBeTrue)
			So(plan.RiskDistance.Sign(), ShouldEqual, 1)
			So(plan.MaxLoss.Sign(), ShouldEqual, 1)

			/*
				The coupling: whatever this entry was sized at, reaching the hard
				floor with it must cost no more than the loss budget. Stop
				distance and quantity are one decision, and a stop wide enough to
				survive noise is only affordable because the size came down to
				meet it.
			*/
			lossPerUnit := plan.LossPerUnit(thesis.Decisions[0].ReferencePrice)
			So(lossPerUnit, ShouldNotBeNil)
			loss := lossPerUnit.Mul(thesis.Decisions[0].ProposedQuantity)
			So(loss.Cmp(plan.MaxLoss), ShouldBeLessThanOrEqualTo, 0)
		})

		Convey("A risk distance the budget cannot carry should shrink the quantity", func() {
			thesis := types.NewThesis()
			thesis.Decisions = []types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionEnter,
				Symbol: "SIM1/USD",
				At:     thesis.At,
			}}

			So(allocator.Allocate(thesis), ShouldBeNil)
			unconstrained := thesis.Decisions[0].ProposedQuantity.Copy()

			/*
				A wide expected spread widens the boundary, which is exactly the
				case where an unchanged size would turn every stopped trade into
				a proportionally larger loss.
			*/
			wide := types.NewThesis()
			wide.Decisions = []types.Decision{{
				ID:             uuid.NewString(),
				Action:         types.ActionEnter,
				Symbol:         "SIM1/USD",
				At:             wide.At,
				ExpectedSpread: decimal.NewFromFloat64(5),
				ExpectedImpact: decimal.NewFromFloat64(1),
				ReferencePrice: decimal.NewFromFloat64(100),
			}}

			So(allocator.Allocate(wide), ShouldBeNil)

			if wide.Decisions[0].Action == types.ActionEnter {
				So(wide.Decisions[0].Risk.RiskDistance.Cmp(
					thesis.Decisions[0].Risk.RiskDistance,
				), ShouldEqual, 1)
				So(wide.Decisions[0].ProposedQuantity.Cmp(unconstrained), ShouldEqual, -1)
			} else {
				// Or the size it would need falls below what the venue will
				// accept, which is a refusal rather than an oversized bet.
				So(wide.Decisions[0].Reason, ShouldEqual,
					"sized quantity below minimum pair order size")
			}
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

		Convey("A published flow haircut should reduce notional before risk sizing", func() {
			clean := types.NewThesis()
			clean.Decisions = []types.Decision{{
				ID:               uuid.NewString(),
				Action:           types.ActionEnter,
				Symbol:           "SIM1/USD",
				At:               clean.At,
				ProposedNotional: decimal.NewFromFloat64(100),
			}}
			So(allocator.Allocate(clean), ShouldBeNil)

			reduced := types.NewThesis()
			reduced.Decisions = []types.Decision{{
				ID:                      uuid.NewString(),
				Action:                  types.ActionEnter,
				Symbol:                  "SIM1/USD",
				At:                      reduced.At,
				ProposedNotional:        decimal.NewFromFloat64(100),
				AllocationHaircut:       0.5,
				AllocationHaircutReason: "toxicity",
			}}
			So(allocator.Allocate(reduced), ShouldBeNil)

			So(reduced.Decisions[0].Action, ShouldEqual, types.ActionEnter)
			So(reduced.Decisions[0].ProposedNotional.Cmp(
				clean.Decisions[0].ProposedNotional,
			), ShouldEqual, -1)
			So(reduced.Decisions[0].ProposedQuantity.Cmp(
				clean.Decisions[0].ProposedQuantity,
			), ShouldEqual, -1)
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
		market.Desk,
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
