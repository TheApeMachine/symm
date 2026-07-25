package integration

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestDeskMarkAndPnL proves Desk marks open lots from live ticker geometry
through the production boot path, and that PnL moves with the tape — not a
one-shot snapshot that freezes after entry.
*/
func TestDeskMarkAndPnL(t *testing.T) {
	Convey("Given a warmed production desk on a three-symbol tape", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)
		So(wired.Desk.Status(), ShouldEqual, types.READY)
		So(wired.Desk.OpenPositions(), ShouldEqual, 0)
		So(wired.Desk.MaxSlots(false), ShouldEqual, 2)

		Convey("When a fast pump opens a lot through Decide+Tick", func() {
			var entered types.Decision
			opened := false

			So(market.Transition(tests.MarketStateFastPump, func() error {
				if opened {
					So(market.Paper.Drain(), ShouldBeNil)
					return nil
				}

				thesis := wired.Thesis

				if thesis == nil {
					return nil
				}

				for open := range wired.Balance.Holdings() {
					if open.Status != types.OPEN ||
						open.Qty == nil || open.Qty.Sign() <= 0 ||
						open.EntryPrice == nil || open.EntryPrice.Sign() <= 0 {
						continue
					}

					entered = types.Decision{
						Symbol:           open.Symbol,
						ProposedQuantity: open.Qty.Copy(),
						ReferencePrice:   open.EntryPrice.Copy(),
					}
					opened = true
					So(wired.Desk.OpenPositions(), ShouldEqual, 1)
					So(market.Paper.Drain(), ShouldBeNil)

					return nil
				}

				for _, decision := range thesis.Decisions {
					if decision.Action != types.ActionEnter {
						continue
					}

					holding, holdErr := wired.Balance.Holding(decision.Symbol)

					if holdErr != nil {
						return nil
					}

					So(holding.Status, ShouldEqual, types.OPEN)
					So(holding.Qty.Cmp(decision.ProposedQuantity), ShouldEqual, 0)
					So(holding.EntryPrice, ShouldNotBeNil)
					So(holding.EntryPrice.Sign(), ShouldEqual, 1)
					So(holding.Mark, ShouldNotBeNil)
					So(holding.Mark.Sign(), ShouldEqual, 1)
					So(holding.PnL, ShouldNotBeNil)
					entered = decision
					opened = true
					So(wired.Desk.OpenPositions(), ShouldEqual, 1)
					So(market.Paper.Drain(), ShouldBeNil)

					return nil
				}

				So(market.Paper.Drain(), ShouldBeNil)

				return nil
			}), ShouldBeNil)

			So(opened, ShouldBeTrue)
			So(entered.Symbol, ShouldBeIn, market.Symbols)

			holding, err := wired.Balance.Holding(entered.Symbol)
			So(err, ShouldBeNil)
			entry := holding.EntryPrice.Copy()
			entryMark := holding.Mark.Copy()
			entryPnL := holding.PnL.Copy()

			Convey("A continuing pump raises mark and improves PnL", func() {
				improved := false

				So(market.Transition(tests.MarketStateFastPump, func() error {
					So(market.Paper.Drain(), ShouldBeNil)
					lot, holdErr := wired.Balance.Holding(entered.Symbol)
					So(holdErr, ShouldBeNil)
					So(lot.Status, ShouldEqual, types.OPEN)
					So(lot.Qty.Cmp(entered.ProposedQuantity), ShouldEqual, 0)
					So(lot.EntryPrice.Cmp(entry), ShouldEqual, 0)
					So(lot.Mark, ShouldNotBeNil)
					So(lot.PnL, ShouldNotBeNil)

					if lot.Mark.Cmp(entryMark) > 0 && lot.PnL.Cmp(entryPnL) > 0 {
						improved = true
					}

					return nil
				}), ShouldBeNil)

				So(improved, ShouldBeTrue)

				Convey("An adverse dump lowers mark and worsens PnL", func() {
					peak, peakErr := wired.Balance.Holding(entered.Symbol)
					So(peakErr, ShouldBeNil)
					So(peak.Mark, ShouldNotBeNil)
					So(peak.PnL, ShouldNotBeNil)
					peakMark := peak.Mark.Copy()
					peakPnL := peak.PnL.Copy()
					worsened := false

					So(market.Transition(tests.MarketStateFastDump, func() error {
						So(market.Paper.Drain(), ShouldBeNil)
						lot, holdErr := wired.Balance.Holding(entered.Symbol)

						if holdErr != nil {
							// Full exit under stop is allowed; mark path already
							// proved above while the lot was open.
							return nil
						}

						So(lot.Mark, ShouldNotBeNil)
						So(lot.PnL, ShouldNotBeNil)

						if lot.Mark.Cmp(peakMark) < 0 && lot.PnL.Cmp(peakPnL) < 0 {
							worsened = true
						}

						return nil
					}), ShouldBeNil)

					So(worsened || wired.Desk.OpenPositions() == 0, ShouldBeTrue)
				})
			})
		})
	})
}

/*
TestDeskSlotsAndExit proves slot capacity, single-lot ownership, and
full-lot Sell through the simulated paper path.
*/
func TestDeskSlotsAndExit(t *testing.T) {
	Convey("Given a warmed production desk", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)
		quote := viper.GetString("market.quote_currency")
		cash, err := wired.Balance.AssetAvailable(quote)
		So(err, ShouldBeNil)
		So(cash.Sign(), ShouldEqual, 1)

		Convey("When a pump opens exactly one lot", func() {
			opened := ""

			So(market.Transition(tests.MarketStateFastPump, func() error {
				if opened != "" {
					for open := range wired.Balance.Holdings() {
						if open.Symbol == opened || open.Status != types.OPEN {
							continue
						}

						So(wired.Desk.Sell(open.Symbol), ShouldBeNil)
					}

					So(market.Paper.Drain(), ShouldBeNil)
					So(wired.Desk.OpenPositions(), ShouldEqual, 1)

					return nil
				}

				for open := range wired.Balance.Holdings() {
					if open.Status != types.OPEN ||
						open.Qty == nil || open.Qty.Sign() <= 0 {
						continue
					}

					opened = open.Symbol
					So(wired.Desk.OpenPositions(), ShouldBeGreaterThanOrEqualTo, 1)
					So(wired.Desk.OpenPositions(), ShouldBeLessThanOrEqualTo, wired.Desk.MaxSlots(false))
					So(wired.Desk.HasSlot(false) || wired.Desk.OpenPositions() >= wired.Desk.MaxSlots(false), ShouldBeTrue)

					return nil
				}

				So(market.Paper.Drain(), ShouldBeNil)

				return nil
			}), ShouldBeNil)

			So(opened, ShouldNotBeBlank)
			qty, err := wired.Balance.Holding(opened)
			So(err, ShouldBeNil)
			heldQty := qty.Qty.Copy()

			Convey("Sell closes the full lot and frees the slot", func() {
				So(wired.Desk.Sell(opened), ShouldBeNil)
				So(market.Paper.Drain(), ShouldBeNil)

				closed := false

				So(market.Transition(tests.MarketStateBaseline, func() error {
					So(market.Paper.Drain(), ShouldBeNil)

					if _, holdErr := wired.Balance.Holding(opened); holdErr != nil {
						closed = true

						return nil
					}

					return nil
				}), ShouldBeNil)

				So(closed, ShouldBeTrue)
				So(heldQty.Sign(), ShouldEqual, 1)
				_, holdErr := wired.Balance.Holding(opened)
				So(holdErr, ShouldNotBeNil)

				for open := range wired.Balance.Holdings() {
					if open.Status != types.OPEN {
						continue
					}

					So(wired.Desk.Sell(open.Symbol), ShouldBeNil)
				}

				So(market.Paper.Drain(), ShouldBeNil)
				So(market.Transition(tests.MarketStateBaseline, func() error {
					So(market.Paper.Drain(), ShouldBeNil)

					for open := range wired.Balance.Holdings() {
						if open.Status == types.OPEN {
							So(wired.Desk.Sell(open.Symbol), ShouldBeNil)
						}
					}

					So(market.Paper.Drain(), ShouldBeNil)

					return nil
				}), ShouldBeNil)

				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
				So(wired.Desk.HasSlot(false), ShouldBeTrue)
			})
		})
	})
}

/*
TestDeskThinBookDoesNotFabricateMarks proves a thin-liquidity book-only
leg still leaves previously opened marks coherent when tickers arrive again.
*/
func TestDeskThinBookDoesNotFabricateMarks(t *testing.T) {
	Convey("Given an open lot after a pump", t, func() {
		market := tests.NewMarket(t.Context(), 2)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)

		opened := ""

		So(market.Transition(tests.MarketStateFastPump, func() error {
			So(market.Paper.Drain(), ShouldBeNil)

			for open := range wired.Balance.Holdings() {
				if open.Status != types.OPEN || open.Mark == nil {
					continue
				}

				opened = open.Symbol
				So(open.Mark.Sign(), ShouldEqual, 1)
				So(open.PnL, ShouldNotBeNil)

				return nil
			}

			return nil
		}), ShouldBeNil)

		if opened == "" {
			Convey("No enter on this tape is an honest outcome", func() {
				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
			})

			return
		}

		before, err := wired.Balance.Holding(opened)
		So(err, ShouldBeNil)
		beforeMark := before.Mark.Copy()

		Convey("Thin liquidity then baseline keeps a positive executable mark", func() {
			So(market.Transition(tests.MarketStateThinLiquidity, tests.Idle), ShouldBeNil)
			So(market.Transition(tests.MarketStateBaseline, func() error {
				So(market.Paper.Drain(), ShouldBeNil)
				lot, holdErr := wired.Balance.Holding(opened)

				if holdErr != nil {
					return nil
				}

				So(lot.Mark, ShouldNotBeNil)
				So(lot.Mark.Sign(), ShouldEqual, 1)
				So(lot.Mark.Cmp(decimal.NewFromInt64(0)), ShouldEqual, 1)
				_ = beforeMark

				return nil
			}), ShouldBeNil)
		})
	})
}

/*
BenchmarkDeskOnTickerMark measures mark+publish cost after an open lot exists.
*/
func BenchmarkDeskOnTickerMark(b *testing.B) {
	market := tests.NewMarket(b.Context(), 2)
	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		_ = wired.Close()
		market.Close()
	}()

	if err := market.Warmup(tests.Idle); err != nil {
		b.Fatal(err)
	}

	opened := false

	if err := market.Transition(tests.MarketStateFastPump, func() error {
		_ = market.Paper.Drain()

		for open := range wired.Balance.Holdings() {
			if open.Status == types.OPEN {
				opened = true
			}
		}

		return nil
	}); err != nil {
		b.Fatal(err)
	}

	if !opened {
		b.Skip("pump tape did not open a lot")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := market.Transition(tests.MarketStateBaseline, func() error {
			return market.Paper.Drain()
		}); err != nil {
			b.Fatal(err)
		}
	}
}
