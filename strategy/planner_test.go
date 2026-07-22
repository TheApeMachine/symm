package strategy_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/* TestUpdate proves production measurements and forecasts reach the Thesis. */
func TestUpdate(t *testing.T) {
	Convey("Given a warmed production graph on a three-symbol tape", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		origin := market.Now()
		var thesis *types.Thesis
		cuts := int64(0)
		consume := func() error {
			var tickErr error
			thesis, tickErr = wired.Crypto.Tick()

			if tickErr != nil {
				return tickErr
			}

			cuts++
			So(thesis, ShouldNotBeNil)
			So(thesis.Tick, ShouldEqual, cuts)
			return nil
		}

		So(market.Warmup(consume), ShouldBeNil)
		So(cuts, ShouldEqual, int64(16))

		Convey("When a calibrated multi-leg fast pump plays through Tick", func() {
			for range 3 {
				So(market.Transition(tests.MarketStateFastPump, consume), ShouldBeNil)
			}

			Convey("Update leaves complete measured evidence and forecasts", func() {
				So(thesis.Incomplete(), ShouldBeFalse)
				So(cuts, ShouldEqual, int64(52))
				So(thesis.Tick, ShouldEqual, int64(52))
				So(thesis.At.Equal(market.Now()), ShouldBeTrue)
				So(thesis.At.Sub(origin), ShouldEqual, 34*time.Second)
				So(len(thesis.Measurements), ShouldEqual, 429)
				currentSources := map[types.SourceType]bool{}

				for _, measurement := range thesis.Measurements {
					So(measurement, ShouldNotBeNil)
					So(measurement.ValidateStruct(), ShouldBeNil)
					So(measurement.Symbol, ShouldBeIn, market.Symbols)
					So(measurement.At.After(thesis.At), ShouldBeFalse)
					from, through := measurement.Interval()
					So(through.Equal(measurement.At), ShouldBeTrue)
					So(from.After(through), ShouldBeFalse)
					So(measurement.Horizon, ShouldEqual, through.Sub(from))
					So(measurement.Scale.From.IsZero(), ShouldEqual,
						measurement.Scale.Through.IsZero())

					if !measurement.Scale.From.IsZero() {
						So(measurement.Scale.Kind, ShouldEqual, types.ScaleObservationWindow)
						So(measurement.Scale.From.After(measurement.Scale.Through), ShouldBeFalse)
						So(measurement.Scale.Through.Equal(measurement.At), ShouldBeTrue)
					}

					if measurement.At.Equal(thesis.At) {
						currentSources[measurement.Source] = true
					}
				}

				So(currentSources, ShouldResemble, map[types.SourceType]bool{
					types.SourceCorrelation: true, types.SourceCVD: true,
					types.SourceDepthFlow: true, types.SourceExhaustion: true,
					types.SourceFluid: true, types.SourceHawkes: true,
					types.SourceLeadLag: true, types.SourceLiquidity: true,
					types.SourcePumpDump: true, types.SourceResonance: true,
					types.SourceSentiment: true, types.SourceToxicity: true,
				})

				for _, value := range thesis.Resonance {
					outcome := value.(*logic.ResonanceOutcome)
					So(outcome.Symbol, ShouldBeIn, market.Symbols)
					So(outcome.ReturnReady, ShouldBeTrue)
				}

				for _, value := range thesis.Causal {
					outcome := value.(*logic.CausalOutcome)
					So(outcome.Symbol, ShouldBeIn, market.Symbols)
					So(outcome.Ready, ShouldBeTrue)
				}

				actualForecasts := map[string]bool{}

				for _, forecast := range thesis.Forecasts {
					So(actualForecasts[forecast.Symbol], ShouldBeFalse)
					actualForecasts[forecast.Symbol] = true
					So(forecast.Symbol, ShouldBeIn, market.Symbols)
					So(forecast.Eligible(), ShouldBeTrue)
					So(forecast.Source, ShouldEqual, "resonance+causal")
					So(forecast.At.Equal(thesis.At), ShouldBeTrue)
					So(forecast.ObservedInterval, ShouldEqual, forecast.At.Sub(origin))
					So(forecast.SourceEpoch, ShouldEqual, uint64(thesis.Tick))
					So(forecast.ExpiresEpoch, ShouldEqual,
						forecast.SourceEpoch+forecast.HorizonEvents)
					So(forecast.Target, ShouldEqual, "next_l3_epoch_mid_log_return")
					So(forecast.ModelVersion, ShouldEqual, "resonance_return_head_v2_rls")
				}

				So(actualForecasts, ShouldResemble, map[string]bool{
					market.Symbols[0]: true,
					market.Symbols[1]: true,
					market.Symbols[2]: true,
				})
			})
		})
	})
}

/* TestDecide proves exact allocation and lifecycle across every market regime. */
func TestDecide(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		Convey("Given a warmed production graph on a three-symbol tape", t, func() {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			Reset(func() {
				So(wired.Close(), ShouldBeNil)
				market.Close()
			})

			So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
			synctest.Wait()

			maxFraction := viper.GetFloat64("trading.allocation.max_fraction")
			So(maxFraction, ShouldEqual, 0.2)
			maxFractionDecimal := decimal.NewFromFloat64(maxFraction)
			So(wired.Desk.OpenPositions(), ShouldEqual, 0)
			So(wired.Desk.MaxSlots(false), ShouldEqual, 2)

			allActions := []types.Action{
				types.ActionEnter, types.ActionHold, types.ActionExit, types.ActionNothing,
			}
			onlyNothing := []types.Action{types.ActionNothing}

			for _, scenario := range []struct {
				name          string
				state         tests.MarketState
				ticks, legs   int
				subject       bool
				rotates       bool
				actions       []types.Action
				entered       []int
				peakPositions int
			}{
				{"baseline", tests.MarketStateBaseline, 16, 1, false, false, nil, nil, 0},
				{"fast dump", tests.MarketStateFastDump, 12, 1, false, false, onlyNothing, nil, 0},
				{"slow dump", tests.MarketStateSlowDump, 20, 1, false, false, onlyNothing, nil, 0},
				{"absorption", tests.MarketStateVolumeAbsorption, 12, 1, true, false, nil, nil, 0},
				{"compression", tests.MarketStateSpreadCompression, 12, 1, true, false, nil, nil, 0},
				{"thin book", tests.MarketStateThinLiquidity, 1, 1, true, false, nil, nil, 0},
				{"loaded book", tests.MarketStateLoadedLiquidity, 18, 1, true, false, nil, nil, 0},
				{"retreat", tests.MarketStateLiquidityRetreat, 1, 1, true, false, nil, nil, 0},
				{"spoof", tests.MarketStateSpoofLiquidity, 1, 1, true, false, nil, nil, 0},
				{"depth thinning", tests.MarketStateDepthThinning, 1, 1, true, false, nil, nil, 0},
				{"spread control", tests.MarketStateSpreadControl, 12, 1, true, false, nil, nil, 0},
				{"fast pump", tests.MarketStateFastPump, 48, 3, false, false, allActions, []int{0, 1}, 2},
				{"slow pump", tests.MarketStateSlowPump, 48, 3, false, true, allActions, []int{0, 1, 2}, 2},
				{"slow cadence", tests.MarketStateSlowCadenceLift, 48, 3, false, false, allActions, []int{0, 1}, 2},
				{"small lift", tests.MarketStateSmallLift, 48, 3, false, false, allActions, []int{0, 1}, 2},
				{"low-volume lift", tests.MarketStateLowVolumeLift, 48, 3, false, false, allActions, []int{0, 1}, 2},
				{"leader follower", tests.MarketStateLeaderFollower, 72, 3, false, false, allActions, []int{0, 1}, 2},
				{"adverse divergence", tests.MarketStateAdverseDivergence, 48, 3, false, false, allActions, []int{1, 2}, 2},
			} {
				Convey("When "+scenario.name+" plays through every Tick", func() {
					open := map[string]*decimal.Decimal{}
					pending := map[string]*decimal.Decimal{}
					entered := map[string]bool{}
					cuts, decisions, peak := 0, 0, 0
					enterSeen, holdSeen, exitSeen, rotationSeen := false, false, false, false
					symbols := []string(nil)

					if scenario.subject {
						symbols = market.Symbols[:1]
					}

					consume := func() error {
						available, availableErr := wired.Balance.AssetAvailable("USD")
						So(availableErr, ShouldBeNil)
						next, tickErr := wired.Crypto.Tick()

						if tickErr != nil {
							return tickErr
						}

						cuts++
						So(next, ShouldNotBeNil)
						SoMsg(scenario.name, next.Incomplete(), ShouldBeFalse)
						synctest.Wait()

						for _, decision := range next.Decisions {
							decisions++
							So(decision.Action, ShouldBeIn, scenario.actions)
							So(decision.Symbol, ShouldBeIn, market.Symbols)
							So(decision.Cause, ShouldNotBeBlank)
							So(decision.Reason, ShouldNotBeBlank)

							switch decision.Action {
							case types.ActionEnter:
								So(decision.ProposedQuantity.Sign(), ShouldEqual, 1)
								So(decision.ProposedNotional.Sign(), ShouldEqual, 1)
								So(decision.AvailableCapital.Sign(), ShouldEqual, 1)
								So(decision.AvailableCapital.Cmp(available), ShouldEqual, 0)
								limit := decimal.ExactMul(
									decision.AvailableCapital,
									maxFractionDecimal,
								)
								allocation := limit

								if decision.Cause == "rotation" {
									rotationSeen = true
									So(decision.Displaces, ShouldNotBeBlank)
									So(decision.DisplacedQuantity, ShouldNotBeNil)
									So(decision.DisplacedPrice, ShouldNotBeNil)
									allocation = decimal.ExactMul(
										decision.DisplacedPrice,
										decision.DisplacedQuantity,
									)
									allocation = allocation.Sub(decimal.ExactMul(
										allocation.Copy(),
										decimal.NewFromFloat64(decision.Alternatives["exit_cost"]),
									))
									So(decision.Reason, ShouldEqual, "sized from displaced capital")
								}

								So(decision.ProposedNotional.Cmp(allocation), ShouldBeLessThanOrEqualTo, 0)
								budget := allocation.Copy().Sub(decimal.ExactMul(
									allocation.Copy(),
									decimal.NewFromFloat64(decision.AllocationHaircut),
								))
								pair, pairErr := wired.Instrument.Pair(decision.Symbol)
								So(pairErr, ShouldBeNil)
								expectedQuantity, quantityErr := wired.Price.Quantity(pair, budget)
								So(quantityErr, ShouldBeNil)
								expectedNotional, notionalErr := wired.Price.Taker(
									pair,
									expectedQuantity,
								)
								So(notionalErr, ShouldBeNil)
								So(decision.ProposedQuantity.Cmp(expectedQuantity), ShouldEqual, 0)
								So(decision.ProposedNotional.Cmp(expectedNotional), ShouldEqual, 0)
								So(decision.SlotCapacity, ShouldEqual, wired.Desk.MaxSlots(false))
								phase, found := next.Lifecycle.Load(decision.Symbol)
								So(found, ShouldBeTrue)

								if decision.Cause == "rotation" {
									So(phase, ShouldEqual, types.LifecycleEntrySelected)
									pending[decision.Symbol] = decision.ProposedQuantity.Copy()
								} else {
									So(phase, ShouldEqual, types.LifecycleEntrySubmitted)
									open[decision.Symbol] = decision.ProposedQuantity.Copy()
								}

								entered[decision.Symbol] = true
								enterSeen = true
							case types.ActionHold:
								quantity, found := open[decision.Symbol]
								So(found, ShouldBeTrue)
								holding, holdErr := wired.Balance.Holding(decision.Symbol)
								So(holdErr, ShouldBeNil)
								So(holding.Qty.Cmp(quantity), ShouldEqual, 0)
								So(holding.Stoploss.Armed(), ShouldBeTrue)
								holdSeen = true
							case types.ActionExit:
								quantity, found := open[decision.Symbol]
								So(found, ShouldBeTrue)
								So(decision.ProposedQuantity.Cmp(quantity), ShouldEqual, 0)
								So(decision.Cause, ShouldBeIn,
									[]string{"stop", "take_profit", "rotation"})
								phase, found := next.Lifecycle.Load(decision.Symbol)
								So(found, ShouldBeTrue)
								So(phase, ShouldBeIn, []string{
									types.LifecycleExitSelected, types.LifecycleExitSubmitted,
								})
								delete(open, decision.Symbol)
								exitSeen = true
							}
						}

						for symbol, quantity := range pending {
							phase, found := next.Lifecycle.Load(symbol)

							if !found || phase != types.LifecycleEntrySubmitted {
								continue
							}

							open[symbol] = quantity
							delete(pending, symbol)
						}

						So(market.Paper.Drain(), ShouldBeNil)
						holdingCount := 0

						for holding := range wired.Balance.Holdings() {
							quantity, found := open[holding.Symbol]
							So(found, ShouldBeTrue)
							So(holding.Qty.Cmp(quantity), ShouldEqual, 0)
							holdingCount++
						}

						So(holdingCount, ShouldEqual, len(open))
						So(wired.Desk.OpenPositions(), ShouldEqual, len(open))

						if holdingCount > peak {
							peak = holdingCount
						}

						return nil
					}

					if scenario.rotates {
						for range 2 {
							So(market.Transition(tests.MarketStateSmallLift, consume), ShouldBeNil)
						}

						So(market.Transition(
							tests.MarketStateFastPump, consume, market.Symbols[2],
						), ShouldBeNil)
					}

					if !scenario.rotates {
						for range scenario.legs {
							So(market.Transition(scenario.state, consume, symbols...), ShouldBeNil)
						}
					}

					if len(scenario.entered) > 0 {
						So(market.Transition(tests.MarketStateFastDump, consume), ShouldBeNil)
					}

					So(cuts, ShouldEqual, scenario.ticks)
					So(wired.Desk.OpenPositions(), ShouldEqual, 0)
					So(pending, ShouldBeEmpty)

					if len(scenario.entered) == 0 {
						So(enterSeen, ShouldBeFalse)
						So(peak, ShouldEqual, 0)

						if scenario.actions == nil {
							So(decisions, ShouldEqual, 0)
						}

						return
					}

					So(enterSeen, ShouldBeTrue)
					So(holdSeen, ShouldBeTrue)
					So(exitSeen, ShouldBeTrue)
					So(peak, ShouldEqual, scenario.peakPositions)
					expectedEntered := map[string]bool{}

					for _, index := range scenario.entered {
						expectedEntered[market.Symbols[index]] = true
					}

					SoMsg(scenario.name, entered, ShouldResemble, expectedEntered)
					SoMsg(scenario.name, rotationSeen, ShouldEqual, scenario.rotates)
				})
			}

		})
	})
}

/* BenchmarkDecide measures production Update and Decide against a pump tape. */
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
			_, tickErr := wired.Crypto.Tick()
			return tickErr
		}); err != nil {
			b.Fatal(err)
		}
	}
}
