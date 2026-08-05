package tests

import (
	"context"
	"slices"
	"testing"
	"time"

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

			So(market != nil, ShouldBeTrue)
			So(market.Public != nil, ShouldBeTrue)
			So(market.Private != nil, ShouldBeTrue)
			So(market.Level3 != nil, ShouldBeTrue)
			So(market.State, ShouldEqual, types.Baseline)
		})
	})
}

func TestMarketTransition(t *testing.T) {
	Convey("Given a market in Baseline state", t, func() {
		symbols := []*types.Symbol{
			types.NewSymbol("SIM1/USD", 100.0, 42),
			types.NewSymbol("SIM2/USD", 100.0, 1337),
		}
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Convey("When transitioning one symbol to FastPump", func() {
			market.Tick()
			peerBefore := market.latest["SIM2/USD"].Timestamp
			err := market.Transition("SIM1/USD", types.FastPump)

			So(err, ShouldBeNil)
			So(market.State, ShouldEqual, types.FastPump)
			So(market.generators["SIM1/USD"].IgnitionArmed(), ShouldBeTrue)
			So(market.generators["SIM2/USD"].IgnitionArmed(), ShouldBeFalse)
			So(market.latest["SIM2/USD"].Timestamp.After(peerBefore), ShouldBeTrue)
			liveBook := market.private.Book("SIM1/USD")
			So(liveBook, ShouldNotBeNil)
			So(liveBook.Bids.Levels, ShouldHaveLength, 1)
			So(liveBook.Asks.Levels, ShouldHaveLength, 1)

			pumped := market.generators["SIM1/USD"].Step()
			baseline := market.generators["SIM2/USD"].Step()

			So(pumped.ChangePct, ShouldBeGreaterThan, baseline.ChangePct)
		})

		Convey("When transitioning an unknown symbol", func() {
			err := market.Transition("UNKNOWN/USD", types.FastPump)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				`market: cannot transition unknown symbol "UNKNOWN/USD"`)
			So(market.State, ShouldEqual, types.Baseline)
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
		estimatedEntryPrice := position.Holding.EntryPrice

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

		entryNoiseLimit := position.Holding.Stoploss.Plan.EntryNoiseBand.Mul(
			decimal.NewFromFloat64(position.Holding.Stoploss.Plan.Multiples.Risk),
		)
		market.Desk.ApplyEvidence(coretypes.StopEvidence{
			Symbol:     position.Holding.Symbol,
			Spread:     entryNoiseLimit.Sub(position.Holding.Stoploss.Plan.TickSize),
			ObservedAt: time.Now().UTC(),
			Present:    true,
		})
		market.Tick()
		positions = slices.Collect(market.Desk.Positions())
		position = positions[0]
		So(position.Status, ShouldEqual, coretypes.OPEN)
		So(position.Holding.Stoploss.TriggerReason, ShouldBeBlank)

		market.Desk.ApplyEvidence(coretypes.StopEvidence{
			Symbol:     position.Holding.Symbol,
			Spread:     entryNoiseLimit.Add(position.Holding.Stoploss.Plan.TickSize),
			ObservedAt: time.Now().UTC(),
			Present:    true,
		})
		market.Tick()

		positions = slices.Collect(market.Desk.Positions())
		So(positions, ShouldHaveLength, 1)
		position = positions[0]
		So(position.Holding.Stoploss.TriggerReason, ShouldEqual,
			coretypes.TriggerExecutionNoiseRegime)
		So(position.ExitOrder.ClOrdId, ShouldNotBeBlank)
		So(position.ExitOrder.Volume, ShouldEqual, "0.25")

		market.Tick()
		market.Tick()
		positions = slices.Collect(market.Desk.Positions())
		position = positions[0]

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
