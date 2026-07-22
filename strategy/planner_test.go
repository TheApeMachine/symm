package strategy_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func tick(wired *stack.Stack) (*types.Thesis, error) {
	thesis, err := wired.Crypto.Tick()

	if errnie.IsPreconditionFailed(err) {
		return thesis, nil
	}

	return thesis, err
}

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

		So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)

		Convey("When a fast pump plays through Tick", func() {
			var thesis *types.Thesis

			So(market.Transition(tests.MarketStateFastPump, func() error {
				next, tickErr := tick(wired)

				if tickErr != nil {
					return tickErr
				}

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

		So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)

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
				if entered {
					return nil
				}

				next, tickErr := tick(wired)

				if tickErr != nil {
					return tickErr
				}

				if next == nil {
					return nil
				}

				So(next.Incomplete(), ShouldBeFalse)

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
					So(decision.OpenPositions, ShouldEqual, 0)
					So(market.Paper.Drain(), ShouldBeNil)

					holding, holdErr := wired.Balance.Holding(decision.Symbol)
					So(holdErr, ShouldBeNil)
					So(holding.Status, ShouldEqual, types.OPEN)
					So(holding.Qty.Cmp(decision.ProposedQuantity), ShouldEqual, 0)
					So(holding.EntryPrice, ShouldNotBeNil)
					So(holding.EntryPrice.Sign(), ShouldEqual, 1)
					So(holding.Stoploss, ShouldNotBeNil)

					phase, found := next.Lifecycle.Load(decision.Symbol)
					So(found, ShouldBeTrue)
					So(phase, ShouldEqual, types.LifecycleEntrySubmitted)

					// One Tick can fill every free slot; keep a single lot so
					// hold/exit proofs stay exact without partial reduces.
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

					next, tickErr := tick(wired)

					if tickErr != nil {
						return tickErr
					}

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

					So(wired.Desk.OpenPositions(), ShouldEqual, 1)
					holding, holdErr := wired.Balance.Holding(enteredSymbol)
					So(holdErr, ShouldBeNil)
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

						next, tickErr := tick(wired)

						if tickErr != nil {
							return tickErr
						}

						if next == nil {
							return nil
						}

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
							So(phase, ShouldBeIn, []string{
								types.LifecycleExitSelected,
								types.LifecycleExitSubmitted,
							})

							So(market.Paper.Drain(), ShouldBeNil)
							exited = true
							return nil
						}

						return nil
					}), ShouldBeNil)

					So(exited, ShouldBeTrue)
					So(wired.Desk.OpenPositions(), ShouldEqual, 0)

					_, holdErr := wired.Balance.Holding(enteredSymbol)
					So(holdErr, ShouldNotBeNil)
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

	if err := market.Warmup(tests.Consume(wired.Crypto.Tick)); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := market.Transition(tests.MarketStateFastPump, func() error {
			_, tickErr := tick(wired)
			return tickErr
		}); err != nil {
			b.Fatal(err)
		}
	}
}
