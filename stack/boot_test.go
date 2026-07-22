package stack_test

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	"github.com/theapemachine/symm/types"
)

/*
TestBooter_Test proves the injected paper Conn closes the real production
Desk, Position, execution, and Balance loop against the simulated market.
*/
func TestBooter_Test(t *testing.T) {
	Convey("Given ambient configuration that conflicts with the test market", t, func() {
		previousBuffer := viper.Get("system.websocket.channel.buffer")
		previousBars := viper.Get("signals.volume_clock_bars_per_day")
		viper.Set("system.websocket.channel.buffer", 1)
		viper.Set("signals.volume_clock_bars_per_day", 37)
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			if wired != nil {
				So(wired.Close(), ShouldBeNil)
			}

			market.Close()
			viper.Set("system.websocket.channel.buffer", previousBuffer)
			viper.Set("signals.volume_clock_bars_per_day", previousBars)
		})

		Convey("The graph should use only its deterministic test configuration", func() {
			So(viper.GetInt("system.websocket.channel.buffer"), ShouldEqual, 4096)
			So(cap(wired.Channel), ShouldEqual, 4096)
			So(viper.GetFloat64("signals.volume_clock_bars_per_day"), ShouldEqual, 0.0)

			viper.Set("cognitive.tick_budget", 0)
			err = wired.Crypto.Run()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "cognitive.tick_budget must be positive")
			So(wired.Close(), ShouldBeNil)
			wired = nil
			So(viper.GetInt("system.websocket.channel.buffer"), ShouldEqual, 1)
			So(viper.GetFloat64("signals.volume_clock_bars_per_day"), ShouldEqual, 37.0)
		})
	})

	Convey("Given the complete production graph on a simulated market", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})
		So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
		symbol := market.Symbols[0]
		quantity := decimal.NewFromInt64(1)

		Convey("A Desk entry fills at the simulated ask and updates inventory", func() {
			pair, err := wired.Instrument.Pair(symbol)
			So(err, ShouldBeNil)
			ticker, err := wired.Price.Get(symbol)
			So(err, ShouldBeNil)
			quoteBefore, err := wired.Balance.AssetAvailable("USD")
			So(err, ShouldBeNil)
			feeFraction, err := wired.Price.Fraction(symbol)
			So(err, ShouldBeNil)
			entryNotional := decimal.ExactMul(ticker.Ask, quantity).SetScale(8)
			entryFee := decimal.ExactMul(entryNotional, feeFraction).SetScale(8)
			position, err := wired.Desk.Buy(
				types.NewHolding(t.Context(), symbol, quantity),
				true,
			)
			So(err, ShouldBeNil)
			So(position.Status(), ShouldEqual, types.PENDING)
			So(market.Paper.Drain(), ShouldBeNil)
			So(position.Status(), ShouldEqual, types.OPEN)
			holding, err := wired.Balance.Holding(symbol)
			So(err, ShouldBeNil)
			So(holding.Qty.Cmp(quantity), ShouldEqual, 0)
			So(holding.EntryPrice.Cmp(ticker.Ask), ShouldEqual, 0)
			So(holding.EntryFee.Cmp(entryFee), ShouldEqual, 0)
			So(holding.EntryAt, ShouldNotBeNil)
			So(*holding.EntryAt, ShouldResemble, market.Now())
			baseAfterEntry, err := wired.Balance.AssetAvailable(pair.Base)
			So(err, ShouldBeNil)
			So(baseAfterEntry.Cmp(quantity), ShouldEqual, 0)
			quoteAfterEntry, err := wired.Balance.AssetAvailable("USD")
			So(err, ShouldBeNil)
			expectedQuoteAfterEntry := quoteBefore.SetScale(max(
				quoteBefore.GetScale(),
				entryNotional.GetScale(),
				entryFee.GetScale(),
			)).
				Sub(entryNotional).
				Sub(entryFee)
			So(quoteAfterEntry.Cmp(expectedQuoteAfterEntry), ShouldEqual, 0)

			Convey("A Desk exit fills at the simulated bid and clears inventory", func() {
				exitNotional := decimal.ExactMul(ticker.Bid, quantity).SetScale(8)
				exitFee := decimal.ExactMul(exitNotional, feeFraction).SetScale(8)
				So(wired.Desk.Sell(symbol), ShouldBeNil)
				So(market.Paper.Drain(), ShouldBeNil)
				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
				So(position.Status(), ShouldEqual, types.CLOSED)
				baseAfterExit, err := wired.Balance.AssetAvailable(pair.Base)
				So(err, ShouldBeNil)
				So(baseAfterExit.Cmp(decimal.NewFromInt64(0)), ShouldEqual, 0)
				quoteAfterExit, err := wired.Balance.AssetAvailable("USD")
				So(err, ShouldBeNil)
				expectedQuoteAfterExit := quoteAfterEntry.SetScale(max(
					quoteAfterEntry.GetScale(),
					exitNotional.GetScale(),
					exitFee.GetScale(),
				)).
					Add(exitNotional).
					Sub(exitFee)
				So(quoteAfterExit.Cmp(expectedQuoteAfterExit), ShouldEqual, 0)
				expectedPnL := exitNotional.SetScale(max(
					exitNotional.GetScale(),
					exitFee.GetScale(),
					entryNotional.GetScale(),
					entryFee.GetScale(),
				)).
					Sub(exitFee).
					Sub(entryNotional).
					Sub(entryFee)
				So(quoteAfterExit.Copy().Sub(quoteBefore).Cmp(expectedPnL), ShouldEqual, 0)

				Convey("A later lot ignores execution replay from the closed order", func() {
					next, err := wired.Desk.Buy(
						types.NewHolding(t.Context(), symbol, quantity),
						true,
					)
					So(err, ShouldBeNil)
					So(market.Paper.Drain(), ShouldBeNil)
					So(next.Status(), ShouldEqual, types.OPEN)

					stale := executionfixture.Frame(executionfixture.Options{
						OrderID:     "PAPER-00002",
						ExecID:      "STALE-CLOSED-ORDER",
						Symbol:      symbol,
						Side:        "sell",
						LastQty:     "1.00000000",
						LastPrice:   "1.00000000",
						Cost:        "1.00000000",
						OrderStatus: "filled",
						OrderType:   "market",
						ExecType:    "trade",
						CumQty:      "1.00000000",
						CumCost:     "1.00000000",
						AvgPrice:    "1.00000000",
						FeeUsdEquiv: "0.10000000",
						Timestamp:   market.Now().Format(time.RFC3339Nano),
					})
					So(market.Paper.Publish("executions", stale), ShouldBeNil)
					So(market.Paper.Drain(), ShouldBeNil)

					holding, err := wired.Balance.Holding(symbol)
					So(err, ShouldBeNil)
					So(holding.Status, ShouldEqual, types.OPEN)
					So(holding.Qty.Cmp(quantity), ShouldEqual, 0)
					So(next.Status(), ShouldEqual, types.OPEN)
				})
			})
		})
	})
}

/*
TestBooter_TickStrategy proves Crypto.Tick → Decide → Desk fill against a
fixture pump tape on the production graph, then exits the filled lot.
*/
func TestBooter_TickStrategy(t *testing.T) {
	Convey("Given a warmed production graph on one exact market tape", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)

		Convey("When a calibrated three-leg fast pump plays through Tick", func() {
			entered := false
			var thesis *types.Thesis

			consume := func() error {
				next, tickErr := wired.Crypto.Tick()

				if tickErr != nil {
					return tickErr
				}

				thesis = next
				So(thesis.Incomplete(), ShouldBeFalse)

				for _, decision := range thesis.Decisions {
					if decision.Action != types.ActionEnter {
						continue
					}

					So(decision.ProposedQuantity, ShouldNotBeNil)
					So(market.Paper.Drain(), ShouldBeNil)
					holding, holdErr := wired.Balance.Holding(decision.Symbol)
					So(holdErr, ShouldBeNil)
					So(holding.Qty.Cmp(decision.ProposedQuantity), ShouldEqual, 0)
					entered = true
				}

				return nil
			}

			for _, state := range []tests.MarketState{
				tests.MarketStateFastPump,
				tests.MarketStateFastPump,
				tests.MarketStateFastPump,
			} {
				So(market.Transition(state, consume), ShouldBeNil)
			}

			Convey("It opens inventory from strategy decisions and can exit it", func() {
				So(thesis, ShouldNotBeNil)
				So(thesis.Tick, ShouldEqual, int64(52))
				So(entered, ShouldBeTrue)
				openPositions := wired.Desk.OpenPositions()
				sold := 0

				for holding := range wired.Balance.Holdings() {
					So(wired.Desk.Sell(holding.Symbol), ShouldBeNil)
					sold++
				}

				So(sold, ShouldEqual, openPositions)
				So(market.Paper.Drain(), ShouldBeNil)
				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
			})

			Convey("Given a fresh exact lot after strategy calibration", func() {
				subject := market.Symbols[0]
				So(wired.Desk.Sell(subject), ShouldBeNil)
				So(market.Paper.Drain(), ShouldBeNil)
				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
				quantity := decimal.NewFromInt64(1)
				seed := types.NewHolding(t.Context(), subject, quantity)
				thesis.Holdings.Store(subject, seed)
				position, err := wired.Desk.Buy(seed, false)
				So(err, ShouldBeNil)
				So(market.Paper.Drain(), ShouldBeNil)
				So(position.Status(), ShouldEqual, types.OPEN)
				So(market.Apply(tests.MarketStep{
					Advance: time.Second,
					Actions: []tests.MarketAction{
						{
							Kind: tests.MarketTrade, Symbol: subject,
							Side: "buy", Qty: quantity.Float64(),
						},
						{
							Kind: tests.MarketRefill, Symbol: subject,
							Side: "sell", Qty: quantity.Float64(),
						},
					},
				}, consume), ShouldBeNil)
				So(thesis.Tick, ShouldEqual, int64(53))
				opened, err := wired.Balance.Holding(subject)
				So(err, ShouldBeNil)
				So(opened.Stoploss.Armed(), ShouldBeTrue)

				for _, proof := range []struct {
					name, reason    string
					state           tests.MarketState
					tick            int64
					retreat         float64
					freeze, stopped bool
				}{
					{
						"bid retreat", "retreat-driven mark; geometry frozen",
						tests.MarketStateLiquidityRetreat, 54, 1, true, false,
					},
					{
						"thin ask", "retreat-driven mark; geometry frozen",
						tests.MarketStateThinLiquidity, 54, 0.999, true, false,
					},
					{
						"spoof addition", "stop live; path intact",
						tests.MarketStateSpoofLiquidity, 54, 0, false, false,
					},
					{
						"sincere fast dump", "mark returned through live stop",
						tests.MarketStateFastDump, 65, 0, false, true,
					},
				} {
					Convey("When "+proof.name+" reaches the open lot", func() {
						actions := []types.Action{}
						exits := 0
						postExit := false
						stop := opened.Stoploss
						stopReturn := stop.StopReturn
						peakReturn := stop.PeakReturn
						lockedFloor := stop.LockedFloor

						So(market.Transition(proof.state, func() error {
							next, tickErr := wired.Crypto.Tick()

							if tickErr != nil {
								return tickErr
							}

							thesis = next

							for _, decision := range thesis.Decisions {
								if decision.Symbol != subject {
									continue
								}

								actions = append(actions, decision.Action)

								if decision.Action == types.ActionNothing {
									So(position.Status(), ShouldEqual, types.CLOSED)
									So(decision.Cause, ShouldEqual, "post_exit_observation")
									So(decision.Reason, ShouldEqual,
										"market has not reclaimed the exited lot boundary")
									So(decision.ReferencePrice.Cmp(seed.StopMark),
										ShouldBeLessThanOrEqualTo, 0)
									So(decision.Alternatives["reclaim_price"],
										ShouldEqual, seed.StopMark.Float64())
									postExit = true
									continue
								}

								So(decision.Action, ShouldBeIn,
									types.ActionHold, types.ActionExit)

								if decision.Action != types.ActionExit {
									continue
								}

								exits++
								So(decision.Cause, ShouldEqual, "stop")
								So(decision.Reason, ShouldEqual, proof.reason)
								So(decision.ProposedQuantity.Cmp(quantity), ShouldEqual, 0)
								current, holdErr := wired.Balance.Holding(subject)
								So(holdErr, ShouldBeNil)
								So(decision.ReferencePrice.Cmp(current.StopMark), ShouldEqual, 0)
							}

							return market.Paper.Drain()
						}, subject), ShouldBeNil)
						So(thesis.Tick, ShouldEqual, proof.tick)

						if proof.stopped {
							So(exits, ShouldEqual, 1)
							So(postExit, ShouldBeTrue)
							So(position.Status(), ShouldEqual, types.CLOSED)
							_, err = wired.Balance.Holding(subject)
							So(err, ShouldNotBeNil)
							So(stop.Action, ShouldEqual, "stop")
							So(stop.MarkReturn, ShouldBeLessThanOrEqualTo, stop.StopReturn)
							return
						}

						So(actions, ShouldResemble, []types.Action{types.ActionHold})
						So(exits, ShouldEqual, 0)
						So(position.Status(), ShouldEqual, types.OPEN)
						current, err := wired.Balance.Holding(subject)
						So(err, ShouldBeNil)
						So(current.Qty.Cmp(opened.Qty), ShouldEqual, 0)
						So(current.EntryPrice.Cmp(opened.EntryPrice), ShouldEqual, 0)
						So(current.EntryFee.Cmp(opened.EntryFee), ShouldEqual, 0)
						So(*current.EntryAt, ShouldResemble, *opened.EntryAt)
						So(stop.Action, ShouldEqual, "hold")
						So(stop.Reason, ShouldEqual, proof.reason)
						So(stop.Retreat, ShouldEqual, proof.retreat)

						if proof.freeze {
							So(stop.StopReturn, ShouldEqual, stopReturn)
							So(stop.PeakReturn, ShouldEqual, peakReturn)
							So(stop.LockedFloor, ShouldEqual, lockedFloor)
						}
					})
				}
			})
		})
	})
}
