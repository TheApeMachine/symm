package strategy_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
enters collects the entry decisions the planner published, which is its actual
product for a pump: a decision to take a slot.

Decisions are read from the planner's published stream rather than from the
thesis, because the planner clears the thesis once a tick is evaluated and
whatever it decided would be gone before a test could look at it.
*/
func enters(decisions []types.Decision) []types.Decision {
	found := make([]types.Decision, 0, len(decisions))

	for _, decision := range decisions {
		if decision.Action == types.ActionEnter {
			found = append(found, decision)
		}
	}

	return found
}

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a thesis that is not yet ready for strategy evaluation", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("Every planner update should still advance the market tick", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			before := market.Thesis.Tick

			market.Planner.Update(market.Thesis)
			So(market.Thesis.Tick, ShouldEqual, before+1)

			market.Planner.Update(market.Thesis)
			So(market.Thesis.Tick, ShouldEqual, before+2)
		}))
	})
}

/*
TestPlannerPumpEntry drives a pump through the full ensemble and holds the
planner to what an entry decision must contain to be executable. A decision
that cannot be sized, priced, or attributed is not a decision.
*/
func TestPlannerPumpEntry(t *testing.T) {
	Convey("Given a market running the full decision ensemble", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
			testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		}

		Convey("When a pump develops from baseline", tests.WithMarket(t, symbols, func(market *tests.Market) {
			for range 64 {
				market.Tick()
			}

			market.Transition(testtypes.FastPump)

			for range 256 {
				market.Tick()
			}

			entries := enters(market.Decisions())

			Convey("The planner should decide to enter the pumping market", func() {
				So(len(entries), ShouldBeGreaterThan, 0)
			})

			Convey("Every entry decision should be executable", func() {
				for _, decision := range entries {
					So(decision.ValidID(), ShouldBeTrue)
					So(decision.Symbol, ShouldBeIn, []string{
						"SIM1/USD", "SIM2/USD", "SIM3/USD",
					})

					// An entry with no size or no price cannot reach the venue.
					So(decision.ProposedQuantity, ShouldNotBeNil)
					So(decision.ProposedQuantity.Sign(), ShouldEqual, 1)
					So(decision.ProposedNotional, ShouldNotBeNil)
					So(decision.ProposedNotional.Sign(), ShouldEqual, 1)
					So(decision.ReferencePrice, ShouldNotBeNil)
					So(decision.ReferencePrice.Sign(), ShouldEqual, 1)

					// Confidence is a probability; utility must be finite.
					So(decision.Confidence, ShouldBeBetweenOrEqual, 0.0, 1.0)
					So(decision.Uncertainty, ShouldBeGreaterThanOrEqualTo, 0.0)
					So(decision.AllocationHaircut, ShouldBeGreaterThanOrEqualTo, 0.0)

					// The planner must say why, so a decision can be audited.
					So(decision.Cause, ShouldNotBeBlank)
					So(decision.Reason, ShouldNotBeBlank)
				}
			})

			Convey("Every entry should clear its own cost of trading", func() {
				for _, decision := range entries {
					So(decision.ExpectedReturn, ShouldNotBeNil)
					So(decision.ExpectedFees, ShouldNotBeNil)
					So(decision.ExpectedSpread, ShouldNotBeNil)
					So(decision.ExpectedImpact, ShouldNotBeNil)

					/*
						Entering is only rational when the expected return
						survives the friction of getting in and out. A system
						that enters below its own costs bleeds by design.
					*/
					friction := decision.ExpectedFees.
						Add(decision.ExpectedSpread).
						Add(decision.ExpectedImpact)

					So(decision.ExpectedReturn.Cmp(friction), ShouldEqual, 1)
					So(decision.Utility, ShouldBeGreaterThan, 0.0)
				}
			})

			Convey("A pump entry should be attributed to the pumpdump signal", func() {
				So(market.Measurements(), ShouldContainKey, "pumpdump")

				for _, decision := range entries {
					So(decision.ForecastSource, ShouldNotBeBlank)
					So(decision.ValidThroughEpoch, ShouldBeGreaterThan, uint64(0))
				}
			})
		}))
	})
}

/*
TestPlannerSlotDiscipline holds the planner to the slot budget. With normal
slots free an entry takes one; once they are gone a pump is exactly the case
the reserve exists for, and a reserve entry must be marked as such rather
than quietly overrunning the normal budget.
*/
func TestPlannerSlotDiscipline(t *testing.T) {
	Convey("Given a market where a pump competes for slots", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
			testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
			testtypes.NewSymbol("SIM4/USD", 100.0, 5150),
		}

		Convey("When every symbol pumps at once", tests.WithMarket(t, symbols, func(market *tests.Market) {
			for range 64 {
				market.Tick()
			}

			market.Transition(testtypes.FastPump)

			for range 256 {
				market.Tick()
			}

			entries := enters(market.Decisions())

			Convey("The planner should report the slot budget it decided against", func() {
				So(len(entries), ShouldBeGreaterThan, 0)

				for _, decision := range entries {
					/*
						Config allots two normal and two reserved slots, so a
						decision is made against a real capacity, never zero.
					*/
					So(decision.SlotCapacity, ShouldEqual, 2)
					So(decision.OpenPositions, ShouldBeGreaterThanOrEqualTo, 0)
					So(decision.OpenPositions, ShouldBeLessThanOrEqualTo, decision.SlotCapacity)
				}
			})

			Convey("Entries should be classed against the slot budget", func() {
				/*
					Class is decided as the budget is spent, so it cannot be
					recovered from the occupancy stamped on a decision: every
					candidate arbitrated together sees the same desk. The
					budget is enforced per round of arbitration, so entries are
					grouped by the tick that produced them.
				*/
				So(len(entries), ShouldBeGreaterThan, 0)
				rounds := map[int64][]types.Decision{}

				for _, decision := range entries {
					So(decision.ArbitrationRound, ShouldBeGreaterThanOrEqualTo, int64(0))
					rounds[decision.ArbitrationRound] = append(rounds[decision.ArbitrationRound], decision)
				}

				So(len(rounds), ShouldBeGreaterThan, 0)

				for _, round := range rounds {
					normal := 0
					reserved := 0

					for _, decision := range round {
						switch decision.AllocationClass {
						case "normal":
							normal++
						case "reserved":
							reserved++
						}
					}

					// No entry may escape unclassified.
					So(normal+reserved, ShouldEqual, len(round))
					So(normal, ShouldBeLessThanOrEqualTo, 2)

					// The reserve is only drawn on once normal capacity is spent.
					if reserved > 0 {
						So(normal, ShouldEqual, 2)
					}
				}
			})

			Convey("A pump arriving with normal slots full should claim a reserve slot", func() {
				contested := make([]types.Decision, 0, len(entries))

				for _, decision := range entries {
					if decision.AllocationClass == "reserved" {
						contested = append(contested, decision)
					}
				}

				So(len(contested), ShouldBeGreaterThan, 0)

				/*
					Pump and dump is the reserve's main use case: the desk is
					already working, and a pump is worth interrupting for. A
					contested entry must therefore be marked as a reserve
					claim, not left classed normal, and not silently dropped.
				*/
				for _, decision := range contested {
					So(decision.Opportunity, ShouldBeTrue)
					So(decision.OpportunityMargin, ShouldBeGreaterThan, 0.0)
				}
			})

			Convey("The planner should never overrun the total slot budget", func() {
				So(len(entries), ShouldBeGreaterThan, 0)

				/*
					Normal plus reserved is four. Positions are committed to the
					desk before order submission, so later rounds must observe
					the slots consumed by earlier rounds.
				*/
				So(len(entries), ShouldBeLessThanOrEqualTo, 4)
			})
		}))
	})
}

/*
TestPlannerPumpReversal follows a pump into its dump. The planner has to stop
wanting the market it just bought, and the exit has to carry the position it
is closing.
*/
func TestPlannerPumpReversal(t *testing.T) {
	Convey("Given a market that pumped and then rolled over", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		}

		/*
			Exits close positions, so the orders behind them have to fill.
			WithFixtureOrders routes them through the fixture transport rather
			than the external paper venue, which does not know the simulated
			symbols this market trades.
		*/
		Convey("When the pump peaks and dumps", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.WithAutoFill()

			for range 64 {
				market.Tick()
			}

			market.Transition(testtypes.FastPump)

			for range 256 {
				market.Tick()
			}

			pumpEntries := len(enters(market.Decisions()))
			So(pumpEntries, ShouldBeGreaterThan, 0)

			market.Transition(testtypes.FastDump)

			for range 256 {
				market.Tick()
			}

			Convey("The planner should stop entering a collapsing market", func() {
				/*
					The dump profile drifts hard negative. Continuing to open
					new longs into it means the entry gate is reading the
					reversal as continuation.

					Decisions accumulate across the run, so what matters is how
					many entries the dump itself produced, not the running
					total the pump already contributed to.
				*/
				dumpEntries := len(enters(market.Decisions())) - pumpEntries

				So(dumpEntries, ShouldBeLessThan, pumpEntries)
			})

			Convey("Exits should identify the position being closed", func() {
				exits := 0

				for _, decision := range market.Decisions() {
					if decision.Action != types.ActionExit {
						continue
					}

					exits++

					So(decision.ValidID(), ShouldBeTrue)
					So(decision.Symbol, ShouldNotBeBlank)
					So(decision.ProposedQuantity, ShouldNotBeNil)
					So(decision.ProposedQuantity.Sign(), ShouldEqual, 1)
					So(decision.ReferencePrice, ShouldNotBeNil)
					So(decision.ReferencePrice.Sign(), ShouldEqual, 1)
					So(decision.Cause, ShouldNotBeBlank)
				}

				So(exits, ShouldBeGreaterThan, 0)
			})
		}))
	})
}
