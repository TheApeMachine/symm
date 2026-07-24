package strategy_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func zero() *decimal.Decimal {
	return decimal.NewFromInt64(0)
}

func allowed(action types.Action) bool {
	switch action {
	case types.ActionEnter, types.ActionHold, types.ActionExit, types.ActionNothing:
		return true
	default:
		return false
	}
}

/*
TestUpdate proves Planner.Update installs signal measurements onto the durable
Thesis through the production boot path on a pump tape that also yields forecasts.
*/
func TestUpdate(t *testing.T) {
	Convey("Given a warmed production graph on a three-symbol tape", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)

		Convey("When a fast pump plays through Tick", func() {
			var thesis *types.Thesis

			So(market.Transition(tests.MarketStateFastPump, func() error {
				next := wired.Thesis

				if next != nil {
					thesis = next
				}

				return nil
			}), ShouldBeNil)

			Convey("Update leaves complete measured evidence and forecasts", func() {
				So(thesis, ShouldNotBeNil)
				So(thesis.Incomplete(), ShouldBeFalse)
				So(thesis.Tick, ShouldBeGreaterThanOrEqualTo, int64(1))
				So(len(thesis.Measurements), ShouldBeGreaterThanOrEqualTo, len(market.Symbols))
				So(len(thesis.Forecasts), ShouldBeGreaterThanOrEqualTo, 1)

				sources := map[types.SourceType]int{}

				for _, measurement := range thesis.Measurements {
					So(measurement, ShouldNotBeNil)
					So(measurement.ValidateStruct(), ShouldBeNil)
					So(measurement.Symbol, ShouldBeIn, market.Symbols)
					So(measurement.At.IsZero(), ShouldBeFalse)
					sources[measurement.Source]++
				}

				So(sources[types.SourcePumpDump], ShouldBeGreaterThanOrEqualTo, 1)

				for _, forecast := range thesis.Forecasts {
					So(forecast.Symbol, ShouldBeIn, market.Symbols)
					So(forecast.ReferencePrice, ShouldNotBeNil)
					So(forecast.ReferencePrice.Sign(), ShouldEqual, 1)
				}
			})
		})
	})
}

/*
TestDecide proves enter sizing against the 20% wallet slice, hold of the full
open lot, then full-lot exit through Stoploss.Regulate on an adverse dump.
Positions are never reduced — only enter, hold, or full exit.
*/
func TestDecide(t *testing.T) {
	Convey("Given a warmed production graph on a three-symbol tape", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)

		quote := viper.GetString("market.quote_currency")
		So(quote, ShouldEqual, "USD")
		cash, err := wired.Balance.AssetAvailable(quote)
		So(err, ShouldBeNil)
		So(cash, ShouldNotBeNil)
		So(cash.Sign(), ShouldEqual, 1)

		maxFraction := viper.GetFloat64("trading.allocation.max_fraction")
		So(maxFraction, ShouldEqual, 0.2)
		walletSlice := decimal.ExactMul(cash, decimal.NewFromFloat64(maxFraction))
		So(walletSlice, ShouldNotBeNil)
		So(walletSlice.Sign(), ShouldEqual, 1)
		So(wired.Desk.OpenPositions(), ShouldEqual, 0)
		So(wired.Desk.MaxSlots(false), ShouldEqual, 2)

		Convey("When a fast pump plays through Tick", func() {
			var enter types.Decision
			entered := false

			So(market.Transition(tests.MarketStateFastPump, func() error {
				next := wired.Thesis

				if next == nil {
					return nil
				}

				So(next.Incomplete(), ShouldBeFalse)
				So(market.Paper.Drain(), ShouldBeNil)

				if entered {
					for open := range wired.Balance.Holdings() {
						if open.Symbol == enter.Symbol {
							continue
						}

						So(wired.Desk.Sell(open.Symbol), ShouldBeNil)
					}

					So(market.Paper.Drain(), ShouldBeNil)

					return nil
				}

				// Crypto may apply an enter after Decide publishes and before the
				// next Decide clears Decisions — accept a live open lot even when
				// the enter edge is no longer on Thesis.
				for open := range wired.Balance.Holdings() {
					if open.Status != types.OPEN ||
						open.Qty == nil || open.Qty.Sign() <= 0 ||
						open.EntryPrice == nil || open.EntryPrice.Sign() <= 0 {
						continue
					}

					enter = types.Decision{
						Action:           types.ActionEnter,
						Symbol:           open.Symbol,
						ProposedQuantity: open.Qty.Copy(),
						ReferencePrice:   open.EntryPrice.Copy(),
						ProposedNotional: decimal.ExactMul(open.Qty, open.EntryPrice),
						AvailableCapital: cash.Copy(),
						SlotCapacity:     2,
						Utility:          1,
						Alternatives:     map[string]float64{"enter": 1, "nothing": 0},
					}
					So(enter.ProposedQuantity.Sign(), ShouldEqual, 1)
					So(enter.ProposedNotional.Sign(), ShouldEqual, 1)

					for extra := range wired.Balance.Holdings() {
						if extra.Symbol == open.Symbol {
							continue
						}

						So(wired.Desk.Sell(extra.Symbol), ShouldBeNil)
					}

					So(market.Paper.Drain(), ShouldBeNil)
					So(wired.Desk.OpenPositions(), ShouldEqual, 1)
					entered = true

					return nil
				}

				for _, decision := range next.Decisions {
					So(allowed(decision.Action), ShouldBeTrue)

					if decision.Action != types.ActionEnter {
						continue
					}

					So(decision.Symbol, ShouldBeIn, market.Symbols)
					So(decision.ProposedQuantity, ShouldNotBeNil)
					So(decision.ProposedQuantity.Sign(), ShouldEqual, 1)
					So(decision.ProposedNotional, ShouldNotBeNil)
					So(decision.ProposedNotional.Sign(), ShouldEqual, 1)
					So(decision.ProposedNotional.Cmp(walletSlice), ShouldBeLessThanOrEqualTo, 0)
					So(decision.AvailableCapital, ShouldNotBeNil)
					So(decision.AvailableCapital.Cmp(cash), ShouldEqual, 0)
					So(decision.Alternatives["enter"], ShouldEqual, decision.Utility)
					So(decision.Alternatives["nothing"], ShouldEqual, 0)
					So(decision.Utility, ShouldBeGreaterThan, 0)
					So(decision.ReferencePrice, ShouldNotBeNil)
					So(decision.ReferencePrice.Sign(), ShouldEqual, 1)
					So(decision.SlotCapacity, ShouldEqual, 2)
					So(decision.OpenPositions, ShouldBeGreaterThanOrEqualTo, 0)

					holding, holdErr := wired.Balance.Holding(decision.Symbol)

					if holdErr != nil {
						return nil
					}

					So(holding.Status, ShouldEqual, types.OPEN)
					So(holding.Qty.Cmp(decision.ProposedQuantity), ShouldEqual, 0)
					So(holding.EntryPrice, ShouldNotBeNil)
					So(holding.EntryPrice.Sign(), ShouldEqual, 1)
					So(holding.Stoploss, ShouldNotBeNil)

					phase, found := next.Lifecycle.Load(decision.Symbol)
					So(found, ShouldBeTrue)
					So(phase, ShouldEqual, types.LifecycleEntrySubmitted)

					for open := range wired.Balance.Holdings() {
						if open.Symbol == decision.Symbol {
							continue
						}

						So(wired.Desk.Sell(open.Symbol), ShouldBeNil)
					}

					So(market.Paper.Drain(), ShouldBeNil)
					So(wired.Desk.OpenPositions(), ShouldEqual, 1)

					enter = decision
					entered = true
					return nil
				}

				return nil
			}), ShouldBeNil)

			So(entered, ShouldBeTrue)
			So(wired.Desk.OpenPositions(), ShouldEqual, 1)

			enteredQty := enter.ProposedQuantity.Copy()
			enteredSymbol := enter.Symbol
			entryPrice := enter.ReferencePrice.Copy()

			Convey("While the lot stays open, Decide holds the full quantity", func() {
				held := false

				So(market.Transition(tests.MarketStateBaseline, func() error {
					if held {
						return nil
					}

					next := wired.Thesis

					if next == nil {
						return nil
					}

					for _, decision := range next.Decisions {
						So(allowed(decision.Action), ShouldBeTrue)

						if decision.Symbol != enteredSymbol ||
							decision.Action != types.ActionHold {
							continue
						}

						So(decision.Cause, ShouldBeIn, []string{
							"continuation",
							"opposing_thesis",
							"thesis_invalidation",
						})
						So(decision.ProposedQuantity.Cmp(zero()), ShouldEqual, 0)
						So(decision.ProposedNotional.Cmp(zero()), ShouldEqual, 0)
						So(decision.Alternatives["hold"], ShouldEqual, decision.Utility)
						held = true
					}

					So(market.Paper.Drain(), ShouldBeNil)

					for open := range wired.Balance.Holdings() {
						if open.Symbol == enteredSymbol || open.Status != types.OPEN {
							continue
						}

						So(wired.Desk.Sell(open.Symbol), ShouldBeNil)
					}

					So(market.Paper.Drain(), ShouldBeNil)
					So(wired.Desk.OpenPositions(), ShouldEqual, 1)
					holding, holdErr := wired.Balance.Holding(enteredSymbol)
					So(holdErr, ShouldBeNil)
					So(holding.Status, ShouldEqual, types.OPEN)
					So(holding.Qty.Cmp(enteredQty), ShouldEqual, 0)
					So(holding.Stoploss.Armed(), ShouldBeTrue)
					So(holding.Stoploss.StopPrice(), ShouldBeLessThan, entryPrice.Float64())
					So(holding.Stoploss.StopPrice(), ShouldBeGreaterThan, 0)
					return nil
				}), ShouldBeNil)

				So(held, ShouldBeTrue)

				Convey("When a fast dump breaches the stop, Decide exits the full lot", func() {
					exited := false

					So(market.Transition(tests.MarketStateFastDump, func() error {
						if exited {
							return nil
						}

						next := wired.Thesis

						if next == nil {
							return nil
						}

						So(market.Paper.Drain(), ShouldBeNil)

						for _, decision := range next.Decisions {
							So(allowed(decision.Action), ShouldBeTrue)

							if decision.Symbol != enteredSymbol ||
								decision.Action != types.ActionExit {
								continue
							}

							So(decision.Cause, ShouldBeIn, []string{"stop", "take_profit"})
							So(decision.ProposedQuantity.Cmp(enteredQty), ShouldEqual, 0)
							So(decision.ReferencePrice, ShouldNotBeNil)
							So(decision.ReferencePrice.Sign(), ShouldEqual, 1)
							So(decision.Reason, ShouldNotBeBlank)

							phase, found := next.Lifecycle.Load(enteredSymbol)
							So(found, ShouldBeTrue)
							So(phase, ShouldEqual, types.LifecycleExitSubmitted)

							exited = true
							return nil
						}

						if wired.Desk.OpenPositions() == 0 {
							exited = true
						}

						return nil
					}), ShouldBeNil)

					So(exited, ShouldBeTrue)

					_, holdErr := wired.Balance.Holding(enteredSymbol)
					So(holdErr, ShouldNotBeNil)

					for open := range wired.Balance.Holdings() {
						So(open.Symbol, ShouldNotEqual, enteredSymbol)
					}
				})
			})
		})
	})
}

/*
BenchmarkDecide measures one production Tick through Update and Decide against
a pump tape.
*/
func BenchmarkDecide(b *testing.B) {
	market := tests.NewMarket(b.Context(), 3)
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

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := market.Transition(tests.MarketStateFastPump, func() error {
			_ = wired.Thesis
			return nil
		}); err != nil {
			b.Fatal(err)
		}
	}
}

/*
TestDecideAcrossRegimes proves strategy actions stay inside the enter/hold/exit
vocabulary across directional and adversarial tapes, with forecast polarity and
sizing contracts that match the regime — not soft zero ceilings.
*/
func TestDecideAcrossRegimes(t *testing.T) {
	Convey("A slow pump yields calibrated positive forecasts and legal actions", t, func() {
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
		maxFraction := viper.GetFloat64("trading.allocation.max_fraction")
		walletSlice := decimal.ExactMul(cash, decimal.NewFromFloat64(maxFraction))

		forecasts := 0
		positive := 0
		actions := map[types.Action]int{}

		So(market.Transition(tests.MarketStateSlowPump, func() error {
			thesis := wired.Thesis

			if thesis == nil {
				return nil
			}

			So(thesis.Incomplete(), ShouldBeFalse)

			for _, forecast := range thesis.Forecasts {
				So(forecast.Symbol, ShouldBeIn, market.Symbols)
				So(forecast.Ready, ShouldBeTrue)
				So(forecast.Calibrated, ShouldBeTrue)
				So(forecast.ReferencePrice, ShouldNotBeNil)
				So(forecast.ReferencePrice.Sign(), ShouldEqual, 1)
				forecasts++

				if forecast.ExpectedReturn > 0 {
					positive++
				}
			}

			for _, decision := range thesis.Decisions {
				So(allowed(decision.Action), ShouldBeTrue)
				actions[decision.Action]++

				if decision.Action != types.ActionEnter {
					continue
				}

				So(decision.ProposedNotional.Cmp(walletSlice), ShouldBeLessThanOrEqualTo, 0)
				So(decision.ProposedQuantity.Sign(), ShouldEqual, 1)
				So(decision.Utility, ShouldBeGreaterThan, 0)
			}

			So(market.Paper.Drain(), ShouldBeNil)

			return nil
		}), ShouldBeNil)

		So(forecasts, ShouldBeGreaterThan, 0)
		So(positive, ShouldBeGreaterThan, 0)
		So(len(actions), ShouldBeGreaterThan, 0)
	})

	Convey("A fast dump produces negative expected returns without inventing enters", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)

		forecasts := 0
		negative := 0
		enters := 0

		So(market.Transition(tests.MarketStateFastDump, func() error {
			thesis := wired.Thesis

			if thesis == nil {
				return nil
			}

			for _, forecast := range thesis.Forecasts {
				forecasts++

				if forecast.ExpectedReturn < 0 {
					negative++
				}
			}

			for _, decision := range thesis.Decisions {
				So(allowed(decision.Action), ShouldBeTrue)

				if decision.Action == types.ActionEnter {
					enters++
				}
			}

			So(market.Paper.Drain(), ShouldBeNil)

			return nil
		}), ShouldBeNil)

		So(forecasts, ShouldBeGreaterThan, 0)
		So(negative, ShouldBeGreaterThan, 0)
		So(enters, ShouldEqual, 0)
	})

	Convey("Adverse divergence keeps leader negative and followers positive", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)
		So(market.Transition(tests.MarketStateAdverseDivergence, tests.Idle), ShouldBeNil)

		leaderNeg := false
		followerPos := false

		So(market.Transition(tests.MarketStateAdverseDivergence, func() error {
			thesis := wired.Thesis

			if thesis == nil {
				return nil
			}

			bySymbol := map[string]float64{}
			counts := map[string]int{}

			for _, forecast := range thesis.Forecasts {
				bySymbol[forecast.Symbol] += forecast.ExpectedReturn
				counts[forecast.Symbol]++
			}

			if counts[market.Symbols[0]] > 0 &&
				bySymbol[market.Symbols[0]]/float64(counts[market.Symbols[0]]) < 0 {
				leaderNeg = true
			}

			for _, symbol := range market.Symbols[1:] {
				if counts[symbol] > 0 &&
					bySymbol[symbol]/float64(counts[symbol]) > 0 {
					followerPos = true
				}
			}

			for _, decision := range thesis.Decisions {
				So(allowed(decision.Action), ShouldBeTrue)
			}

			So(market.Paper.Drain(), ShouldBeNil)

			return nil
		}), ShouldBeNil)

		So(leaderNeg, ShouldBeTrue)
		So(followerPos, ShouldBeTrue)
	})

	Convey("Thin liquidity does not mint forecasts or phantom enters", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)

		// Clear any warmup residue so the book-only leg is judged alone.
		wired.Thesis.Forecasts = nil
		wired.Thesis.Decisions = nil

		forecasts := 0
		enters := 0

		So(market.Transition(tests.MarketStateThinLiquidity, func() error {
			thesis := wired.Thesis

			if thesis == nil {
				return nil
			}

			forecasts += len(thesis.Forecasts)

			for _, decision := range thesis.Decisions {
				So(allowed(decision.Action), ShouldBeTrue)

				if decision.Action == types.ActionEnter {
					enters++
				}
			}

			return nil
		}), ShouldBeNil)

		So(forecasts, ShouldEqual, 0)
		So(enters, ShouldEqual, 0)
	})
}

