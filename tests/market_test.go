package tests

import (
	"context"
	"slices"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests/types"
	coretypes "github.com/theapemachine/symm/types"
)

func TestMarketNewMarket(t *testing.T) {
	Convey("Given a list of symbols", t, func() {
		symbols := []*types.Symbol{
			types.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("When NewMarket is initialized", func() {
			market := NewMarket(t.Context(), symbols)
			defer market.Close()

			So(market, ShouldNotBeNil)
			So(market.Public, ShouldNotBeNil)
			So(market.Private, ShouldNotBeNil)
			So(market.Level3, ShouldNotBeNil)
			So(market.State, ShouldEqual, types.Baseline)
		})
	})
}

func TestMarketTransition(t *testing.T) {
	Convey("Given a market in Baseline state", t, func() {
		symbols := []*types.Symbol{
			types.NewSymbol("SIM1/USD", 100.0, 42),
		}
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Convey("When transitioning to FastPump", func() {
			market.Transition(types.FastPump)

			So(market.State, ShouldEqual, types.FastPump)
		})
	})
}

func TestMarketWithMarket(t *testing.T) {
	Convey("Given WithMarket test wrapper", t, func() {
		symbols := []*types.Symbol{
			types.NewSymbol("SIM1/USD", 100.0, 42),
		}

		WithMarket(t, symbols, func(market *Market) {
			So(market, ShouldNotBeNil)
			market.Tick()
		})()
	})
}

func TestMarketWithAutoFill(t *testing.T) {
	symbols := []*types.Symbol{
		types.NewSymbol("SIM1/USD", 105.0, 42),
	}

	Convey("Given an executable position lifecycle at the simulated venue", t, WithFixtureOrders(t, symbols, func(market *Market) {
		market.WithAutoFill()
		market.Tick()
		initialSlots := market.Desk.OpenSlots(false)
		entry := coretypes.Decision{
			ID:               "entry-one",
			Action:           coretypes.ActionEnter,
			Symbol:           symbols[0].Pair,
			ProposedQuantity: decimal.NewFromFloat64(0.25),
			Risk:             EntryRisk(market, symbols[0].Pair),
		}

		So(market.Desk.Execute([]coretypes.Decision{entry}), ShouldBeNil)
		So(market.Desk.OpenPositions(), ShouldEqual, 1)
		So(market.Desk.OpenSlots(false), ShouldEqual, initialSlots-1)
		positions := slices.Collect(market.Desk.Positions())

		So(positions, ShouldHaveLength, 1)
		position := positions[0]
		So(position.Holding.EntryPrice, ShouldNotBeNil)
		So(position.Holding.EntryFee, ShouldNotBeNil)
		So(position.Holding.EntryFee.Sign(), ShouldEqual, 1)
		So(position.Holding.Mark.Cmp(position.Holding.EntryPrice), ShouldEqual, -1)
		So(position.Holding.Stoploss, ShouldNotBeNil)
		So(position.Holding.Stoploss.Entry.Cmp(position.Holding.EntryPrice), ShouldEqual, 0)
		So(position.Holding.Stoploss.Mark.Cmp(position.Holding.Mark), ShouldEqual, 0)
		So(position.Holding.Stoploss.Floor, ShouldNotBeNil)
		estimatedEntryPrice := position.Holding.EntryPrice.Copy()

		market.Tick()
		market.Tick()
		positions = slices.Collect(market.Desk.Positions())

		So(positions, ShouldHaveLength, 1)
		position = positions[0]
		So(position.Status, ShouldEqual, coretypes.OPEN)
		So(position.Holding.SellableQty.String(), ShouldEqual, "0.25")
		So(position.Holding.EntryPrice.Float64(), ShouldEqual, 105.0)
		So(position.Holding.EntryPrice.Cmp(estimatedEntryPrice), ShouldNotEqual, 0)
		So(position.Holding.Stoploss.Entry.Cmp(position.Holding.EntryPrice), ShouldEqual, 0)
		So(position.Holding.Stoploss.Mark.Cmp(position.Holding.Mark), ShouldEqual, 0)

		position.Holding.Stoploss.Status = coretypes.TRIGGERED
		market.Tick()

		positions = slices.Collect(market.Desk.Positions())
		So(positions, ShouldHaveLength, 1)
		position = positions[0]
		So(position.ExitOrder.ClOrdId, ShouldNotBeBlank)
		So(position.ExitOrder.Volume, ShouldEqual, "0.25")

		market.Tick()
		market.Tick()

		So(position.Status, ShouldEqual, coretypes.CLOSED)
		So(position.Holding.Status, ShouldEqual, coretypes.CLOSED)
		So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
		So(position.Holding.ExitAt, ShouldNotBeNil)
		So(market.Desk.OpenPositions(), ShouldEqual, 0)
		So(market.Desk.OpenSlots(false), ShouldEqual, initialSlots)

		reentry := coretypes.Decision{
			ID:               "entry-two",
			Action:           coretypes.ActionEnter,
			Symbol:           symbols[0].Pair,
			ProposedQuantity: decimal.NewFromFloat64(0.20),
			Risk:             EntryRisk(market, symbols[0].Pair),
		}
		So(market.Desk.Execute([]coretypes.Decision{reentry}), ShouldBeNil)
		So(market.Desk.OpenPositions(), ShouldEqual, 1)

		positions = slices.Collect(market.Desk.Positions())
		So(positions, ShouldHaveLength, 1)
		So(positions[0].ID, ShouldEqual, reentry.ID)
		So(positions[0].Status, ShouldEqual, coretypes.PENDING)
	}))
}

func BenchmarkMarketTick(b *testing.B) {
	symbols := []*types.Symbol{
		types.NewSymbol("SIM1/USD", 100.0, 42),
		types.NewSymbol("SIM2/USD", 200.0, 43),
	}
	market := NewMarket(context.Background(), symbols)
	defer market.Close()

	for b.Loop() {
		market.Tick()
	}
}
