package strategy_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/stack"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func TestAllocatorAllocate(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
	}

	Convey(
		"Given a market with positive cash balance",
		t, stack.WithOrders(t, symbols, func(market *tests.Market, system *cmd.System) {
			for range 16 {
				market.Tick()
			}

			allocator := strategy.NewAllocator(
				context.Background(),
				system.Desk.Balance(),
				system.Desk.Instrument(),
				system.Desk.Price(),
				system.Desk,
			)

			Convey("An entry decision should be sized and marked ready", func() {
				thesis := types.NewThesis(nil)
				stage(thesis, types.Decision{
					ID:     uuid.NewString(),
					Action: types.ActionEnter,
					Symbol: "SIM1/USD",
					At:     thesis.At,
				})

				err := allocator.Allocate(thesis)
				So(err, ShouldBeNil)
				So(thesis.Allocator, ShouldBeTrue)
				So(only(thesis).Action, ShouldEqual, types.ActionEnter)
				So(only(thesis).ProposedQuantity, ShouldNotBeNil)
				So(only(thesis).ProposedQuantity.Sign(), ShouldBeGreaterThan, 0)
				So(only(thesis).ProposedNotional, ShouldNotBeNil)
				So(only(thesis).ProposedNotional.Sign(), ShouldBeGreaterThan, 0)
				So(only(thesis).ReferencePrice, ShouldNotBeNil)
			})

			Convey("A sized entry should carry the geometry it was sized under", func() {
				thesis := types.NewThesis(nil)
				stage(thesis, types.Decision{
					ID:     uuid.NewString(),
					Action: types.ActionEnter,
					Symbol: "SIM1/USD",
					At:     thesis.At,
				})

				So(allocator.Allocate(thesis), ShouldBeNil)

				plan := only(thesis).Risk
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
				lossPerUnit := plan.LossPerUnit(only(thesis).ReferencePrice)
				So(lossPerUnit, ShouldNotBeNil)
				loss := lossPerUnit.Mul(only(thesis).ProposedQuantity)
				So(loss.Cmp(plan.MaxLoss), ShouldBeLessThanOrEqualTo, 0)
			})

			Convey("A risk distance the budget cannot carry should shrink the quantity", func() {
				thesis := types.NewThesis(nil)
				stage(thesis, types.Decision{
					ID:     uuid.NewString(),
					Action: types.ActionEnter,
					Symbol: "SIM1/USD",
					At:     thesis.At,
				})

				So(allocator.Allocate(thesis), ShouldBeNil)
				unconstrained := only(thesis).ProposedQuantity

				/*
					A wide expected spread widens the boundary, which is exactly the
					case where an unchanged size would turn every stopped trade into
					a proportionally larger loss.
				*/
				wide := types.NewThesis(nil)
				stage(wide, types.Decision{
					ID:             uuid.NewString(),
					Action:         types.ActionEnter,
					Symbol:         "SIM1/USD",
					At:             wide.At,
					ExpectedSpread: decimal.NewFromFloat64(5),
					ExpectedImpact: decimal.NewFromFloat64(1),
					ReferencePrice: decimal.NewFromFloat64(100),
				})

				So(allocator.Allocate(wide), ShouldBeNil)

				if only(wide).Action == types.ActionEnter {
					So(only(wide).Risk.RiskDistance.Cmp(
						only(thesis).Risk.RiskDistance,
					), ShouldEqual, 1)
					So(only(wide).ProposedQuantity.Cmp(unconstrained), ShouldEqual, -1)
				} else {
					// Or the size it would need falls below what the venue will
					// accept, which is a refusal rather than an oversized bet.
					So(only(wide).Reason, ShouldEqual,
						"sized quantity below minimum pair order size")
				}
			})

			Convey("An unconfigured pair should be rejected in place", func() {
				thesis := types.NewThesis(nil)
				stage(thesis, types.Decision{
					ID:     uuid.NewString(),
					Action: types.ActionEnter,
					Symbol: "UNKNOWN/USD",
					At:     thesis.At,
				})

				err := allocator.Allocate(thesis)
				So(err, ShouldBeNil)
				So(only(thesis).Action, ShouldEqual, types.ActionNothing)
				So(only(thesis).Reason, ShouldEqual, "instrument pair unavailable")
			})

			Convey("A published flow haircut should reduce notional before risk sizing", func() {
				clean := types.NewThesis(nil)
				stage(clean, types.Decision{
					ID:               uuid.NewString(),
					Action:           types.ActionEnter,
					Symbol:           "SIM1/USD",
					At:               clean.At,
					ProposedNotional: decimal.NewFromFloat64(100),
				})
				So(allocator.Allocate(clean), ShouldBeNil)

				reduced := types.NewThesis(nil)
				stage(reduced, types.Decision{
					ID:                      uuid.NewString(),
					Action:                  types.ActionEnter,
					Symbol:                  "SIM1/USD",
					At:                      reduced.At,
					ProposedNotional:        decimal.NewFromFloat64(100),
					AllocationHaircut:       0.5,
					AllocationHaircutReason: "toxicity",
				})
				So(allocator.Allocate(reduced), ShouldBeNil)

				So(only(reduced).Action, ShouldEqual, types.ActionEnter)
				So(only(reduced).ProposedNotional.Cmp(
					only(clean).ProposedNotional,
				), ShouldEqual, -1)
				So(only(reduced).ProposedQuantity.Cmp(
					only(clean).ProposedQuantity,
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

	public, private := market.Feeds()
	system := cmd.Boot(b.Context(), types.NewThesis(nil), public, private, nil)

	defer system.Close()

	for range 16 {
		market.Tick()
	}

	allocator := strategy.NewAllocator(
		context.Background(),
		system.Desk.Balance(),
		system.Desk.Instrument(),
		system.Desk.Price(),
		system.Desk,
	)

	thesis := types.NewThesis(nil)
	stage(thesis, types.Decision{
		ID:     uuid.NewString(),
		Action: types.ActionEnter,
		Symbol: "SIM1/USD",
		At:     thesis.At,
	})

	for b.Loop() {
		only(thesis).Action = types.ActionEnter
		only(thesis).ProposedQuantity = nil
		only(thesis).ProposedNotional = nil
		_ = allocator.Allocate(thesis)
	}
}
