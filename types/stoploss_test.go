package types_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
The path this fixture replays is the one a plain percentage trail could not
survive: an immediate adverse move on entry, a recovery through break-even, new
highs, and a price that wicks hard in both directions the whole way. The old
regulator ended it during the first move; every assertion here is a way that
must not happen again.
*/

const fixtureSymbol = "SIM1/USD"

/*
fixtureEntry is a lot bought at 100 with a realized 0.26% taker fee, quoted on a
one-cent tick with a ten-cent spread. Its execution-noise band is therefore 12
cents, which is what every distance below is a multiple of.
*/
func fixtureEntry() (*types.Stoploss, types.RiskPlan) {
	plan := types.NewRiskPlan(types.RiskInputs{
		ReferencePrice: decimal.NewFromFloat64(100),
		Spread:         decimal.NewFromFloat64(0.10),
		Impact:         decimal.NewFromFloat64(0.02),
		TickSize:       decimal.NewFromFloat64(0.01),
		ExitFeeRate:    decimal.NewFromFloat64(0.0026),
		EntryFeeRate:   decimal.NewFromFloat64(0.0026),
		MaxLoss:        decimal.NewFromFloat64(5),
		Multiples:      types.DefaultRiskMultiples(),
	})

	stoploss := types.NewStoploss(
		context.Background(),
		fixtureSymbol,
		decimal.NewFromFloat64(100),
		decimal.NewFromFloat64(1),
		decimal.NewFromFloat64(0.26),
		decimal.NewFromFloat64(99.90),
		plan,
	)

	stoploss.RebindFill(types.Fill{
		EntryPrice: decimal.NewFromFloat64(100),
		EntryFee:   decimal.NewFromFloat64(0.26),
		Qty:        decimal.NewFromFloat64(1),
	})

	return stoploss, plan
}

/*
observe feeds one executable mark, optionally from a book whose bid is being
pulled rather than filled.
*/
func observe(stoploss *types.Stoploss, price float64, hollow bool) {
	evidence := types.StopEvidence{
		Symbol:         fixtureSymbol,
		ExecutableMark: decimal.NewFromFloat64(price),
		Present:        true,
	}

	if hollow {
		evidence.HollowReady = true
		evidence.HollowPressure = 0.4
		evidence.ObservedAt = time.Now().UTC()
	}

	stoploss.Observe(evidence)
}

/*
realized is what selling the whole lot at a price actually nets after the exit
fee and the entry cost it has to cover.
*/
func realized(price *decimal.Decimal) float64 {
	proceeds := price.SetScale(12).Mul(decimal.NewFromFloat64(0.9974))

	return proceeds.Sub(decimal.NewFromFloat64(100.26)).Float64()
}

func TestStoplossGeometry(t *testing.T) {
	Convey("Given a lot filled at 100 with a 12 cent execution-noise band", t, func() {
		stoploss, plan := fixtureEntry()

		Convey("The derived lines should be ordered and separated", func() {
			So(stoploss.HardFloor.Cmp(stoploss.Entry), ShouldEqual, -1)
			So(stoploss.ProfitLine.Cmp(stoploss.Entry), ShouldEqual, 1)
			So(stoploss.ProfitFloor.Cmp(stoploss.ProfitLine), ShouldEqual, 1)
			So(stoploss.ArmLine.Cmp(stoploss.ProfitFloor), ShouldEqual, 1)

			// The whole point of the two buffers: arming and firing cannot
			// happen on the same tick.
			So(plan.ArmBuffer.Cmp(plan.LockBuffer), ShouldEqual, 1)
			So(plan.LockBuffer.Sign(), ShouldEqual, 1)
		})

		Convey("The profit line should be net-positive after the exit fee", func() {
			So(realized(stoploss.ProfitLine), ShouldBeGreaterThan, 0)
		})

		Convey("The hard floor should sit outside ordinary crossing cost", func() {
			distance := stoploss.Entry.Sub(stoploss.HardFloor)
			So(distance.Cmp(plan.NoiseBand), ShouldEqual, 1)
		})

		Convey("The position should be sized so the hard floor costs no more than the loss budget", func() {
			entry := decimal.NewFromFloat64(100)
			capacity := plan.MaxQuantity(entry)
			So(capacity, ShouldNotBeNil)

			/*
				The budget has to bound the *net* loss, not the price move. An
				earlier version capped on the raw distance alone and let both
				crossings ride on top of it, which on a tight stop with a taker
				fee was more than double the budget it claimed to enforce.
			*/
			lossPerUnit := plan.LossPerUnit(entry)
			So(lossPerUnit, ShouldNotBeNil)
			So(lossPerUnit.Cmp(plan.RiskDistance), ShouldEqual, 1)
			So(lossPerUnit.SetScale(12).Mul(capacity).Cmp(plan.MaxLoss),
				ShouldBeLessThanOrEqualTo, 0)
		})
	})
}

func TestStoplossRefusesUnconfirmedBasis(t *testing.T) {
	Convey("Given an order that has been submitted but not filled", t, func() {
		plan := types.NewRiskPlan(types.RiskInputs{
			ReferencePrice: decimal.NewFromFloat64(100),
			Spread:         decimal.NewFromFloat64(0.10),
			Impact:         decimal.NewFromFloat64(0.02),
			TickSize:       decimal.NewFromFloat64(0.01),
			ExitFeeRate:    decimal.NewFromFloat64(0.0026),
			EntryFeeRate:   decimal.NewFromFloat64(0.0026),
			MaxLoss:        decimal.NewFromFloat64(5),
			Multiples:      types.DefaultRiskMultiples(),
		})

		stoploss := types.NewStoploss(
			context.Background(), fixtureSymbol,
			decimal.NewFromFloat64(100), decimal.NewFromFloat64(1),
			decimal.NewFromFloat64(0.26), decimal.NewFromFloat64(99.90),
			plan,
		)

		Convey("The lines exist for sizing but nothing may act on them yet", func() {
			So(stoploss.BasisConfirmed, ShouldBeFalse)
			So(stoploss.Status, ShouldEqual, types.PENDING)
			So(stoploss.HardFloor, ShouldNotBeNil)
		})

		Convey("Ticks arriving before the fill should move nothing", func() {
			/*
				This is the hole. A market buy that takes a few seconds to fill
				sees ticks the whole time, and a run above the arm line during
				that window used to arm protection — so the lot opened already
				defending a profit that belonged to a price it never paid.
			*/
			observe(stoploss, 100.40, false)
			observe(stoploss, 101.50, false)
			observe(stoploss, 98.00, false)

			So(stoploss.Peak, ShouldBeNil)
			So(stoploss.ProfitArmed, ShouldBeFalse)
			So(stoploss.Status, ShouldEqual, types.PENDING)
			So(stoploss.TriggerReason, ShouldBeBlank)
		})

		Convey("The fill should confirm the basis and start the path from there", func() {
			observe(stoploss, 101.50, false)

			stoploss.RebindFill(types.Fill{
				EntryPrice: decimal.NewFromFloat64(100.40),
				EntryFee:   decimal.NewFromFloat64(0.26),
				Qty:        decimal.NewFromFloat64(1),
			})

			So(stoploss.BasisConfirmed, ShouldBeTrue)
			So(stoploss.Status, ShouldEqual, types.ARMED)
			So(stoploss.Peak, ShouldBeNil)
			So(stoploss.ProfitArmed, ShouldBeFalse)
			So(stoploss.Entry.Cmp(decimal.NewFromFloat64(100.40)), ShouldEqual, 0)

			Convey("And marks after it should be honoured", func() {
				observe(stoploss, 100.60, false)
				So(stoploss.Peak.Cmp(decimal.NewFromFloat64(100.60)), ShouldEqual, 0)
			})
		})
	})
}

func TestStoplossProfitFailSafe(t *testing.T) {
	Convey("Given a protected lot whose price collapses through its floor", t, func() {
		stoploss, _ := fixtureEntry()

		observe(stoploss, 101.60, false)
		So(stoploss.ProfitArmed, ShouldBeTrue)

		Convey("The fail-safe should sit between break-even and the giveback floor", func() {
			So(stoploss.ProfitFailSafe.Cmp(stoploss.BreakEvenLine), ShouldEqual, 1)
			So(stoploss.ProfitFailSafe.Cmp(stoploss.Floor), ShouldEqual, -1)
		})

		Convey("A collapse should not spend the whole confirmation window", func() {
			/*
				Confirmation is what makes a wick survivable, and it costs marks.
				On a fast reversal those marks used to carry the position back
				through the price at which the round trip stops paying, and the
				exit landed below break-even having been triggered to protect a
				profit.
			*/
			observe(stoploss, stoploss.ProfitFailSafe.Sub(
				decimal.NewFromFloat64(0.01),
			).Float64(), false)

			So(stoploss.Status, ShouldEqual, types.TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, types.TriggerProfitFailSafe)

			Convey("And the mark it fired on should still be above break-even", func() {
				So(stoploss.Mark.Cmp(stoploss.BreakEvenLine), ShouldEqual, 1)
				So(realized(stoploss.Mark), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestStoplossDepthLimitedMarks(t *testing.T) {
	Convey("Given a book too thin to absorb the position", t, func() {
		stoploss, _ := fixtureEntry()

		observe(stoploss, 100.20, false)
		peak := stoploss.Peak.Copy()

		Convey("A high on liquidity that cannot take the lot builds no geometry", func() {
			stoploss.Observe(types.StopEvidence{
				Symbol:         fixtureSymbol,
				ExecutableMark: decimal.NewFromFloat64(102.00),
				DepthLimited:   true,
				Present:        true,
			})

			So(stoploss.Peak.Cmp(peak), ShouldEqual, 0)
			So(stoploss.ProfitArmed, ShouldBeFalse)
		})

		Convey("But the loss boundary still judges it", func() {
			/*
				A book too thin to absorb the position is evidence of more
				danger, not less. Suspending the hard floor exactly when
				liquidity is disappearing is the opposite of what it is for.
			*/
			stoploss.Observe(types.StopEvidence{
				Symbol:         fixtureSymbol,
				ExecutableMark: stoploss.HardFloor.Sub(decimal.NewFromFloat64(0.01)),
				DepthLimited:   true,
				Present:        true,
			})

			So(stoploss.Status, ShouldEqual, types.TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, types.TriggerHardRisk)
		})
	})
}

func TestStoplossDiscoveryPhase(t *testing.T) {
	Convey("Given a lot that moves against its entry immediately", t, func() {
		stoploss, plan := fixtureEntry()

		Convey("A drawdown above the hard floor should not end the trade", func() {
			observe(stoploss, 99.80, false)
			observe(stoploss, 99.72, false)
			observe(stoploss, 99.68, false)

			So(stoploss.Status, ShouldEqual, types.ARMED)
			So(stoploss.Phase, ShouldEqual, types.PhaseDiscovery)
			So(stoploss.TriggerReason, ShouldBeBlank)
			So(stoploss.Floor.Cmp(stoploss.HardFloor), ShouldEqual, 0)
		})

		Convey("A peak below break-even should still be recorded", func() {
			observe(stoploss, 99.70, false)
			observe(stoploss, 99.95, false)
			observe(stoploss, 99.75, false)

			So(stoploss.Peak.Cmp(decimal.NewFromFloat64(99.95)), ShouldEqual, 0)

			// Recorded, but not yet allowed to exit: 99.95 − 24 cents is above
			// nothing the position has earned.
			So(stoploss.ProfitArmed, ShouldBeFalse)
			So(stoploss.Floor.Cmp(stoploss.HardFloor), ShouldEqual, 0)
			So(stoploss.Status, ShouldEqual, types.ARMED)
		})

		Convey("A breach of the hard floor should fire on sight", func() {
			observe(stoploss, 99.90, false)

			breach := stoploss.HardFloor.Sub(plan.TickSize)
			stoploss.Observe(types.StopEvidence{
				Symbol:         fixtureSymbol,
				ExecutableMark: breach,
				Present:        true,
			})

			So(stoploss.Status, ShouldEqual, types.TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, types.TriggerHardRisk)
		})
	})
}

func TestStoplossProfitProtection(t *testing.T) {
	Convey("Given a lot that recovers and makes new highs", t, func() {
		stoploss, plan := fixtureEntry()

		observe(stoploss, 99.70, false)
		observe(stoploss, 100.10, false)
		observe(stoploss, 100.45, false)

		Convey("Protection should not arm before the arm line", func() {
			So(stoploss.ProfitArmed, ShouldBeFalse)
			So(stoploss.Phase, ShouldEqual, types.PhaseDiscovery)
		})

		Convey("Crossing the arm line should lock a floor strictly above the profit line", func() {
			armed := stoploss.ArmLine.Add(plan.TickSize)
			stoploss.Observe(types.StopEvidence{
				Symbol:         fixtureSymbol,
				ExecutableMark: armed,
				Present:        true,
			})

			So(stoploss.ProfitArmed, ShouldBeTrue)
			So(stoploss.Phase, ShouldEqual, types.PhaseProtected)
			So(stoploss.Floor.Cmp(stoploss.ProfitLine), ShouldEqual, 1)

			// Arming must not also fire: the mark that armed protection has to
			// survive the floor it just installed.
			So(stoploss.Status, ShouldEqual, types.ARMED)
			So(armed.Cmp(stoploss.Floor), ShouldEqual, 1)
		})

		Convey("Successive executable peaks should ratchet the protected floor upward", func() {
			armed := stoploss.ArmLine.Add(plan.TickSize)
			stoploss.Observe(types.StopEvidence{
				Symbol:         fixtureSymbol,
				ExecutableMark: armed,
				Present:        true,
			})

			previousFloor := stoploss.Floor.Copy()
			pullbackDistance := plan.TrailDistance.Div(decimal.NewFromInt64(2))

			for range 4 {
				peak := previousFloor.Add(plan.TrailDistance).Add(plan.TickSize)
				observe(stoploss, peak.Float64(), false)

				So(stoploss.Peak.Cmp(peak), ShouldEqual, 0)
				So(stoploss.Floor.Cmp(previousFloor), ShouldEqual, 1)
				So(stoploss.Floor.Cmp(stoploss.ProfitLine), ShouldEqual, 1)

				ratchetedFloor := stoploss.Floor.Copy()
				observe(stoploss, peak.Sub(pullbackDistance).Float64(), false)

				So(stoploss.Status, ShouldEqual, types.ARMED)
				So(stoploss.Floor.Cmp(ratchetedFloor), ShouldEqual, 0)
				previousFloor = ratchetedFloor
			}
		})

		Convey("An isolated wick should not end a protected trade", func() {
			observe(stoploss, 101.20, false)
			So(stoploss.ProfitArmed, ShouldBeTrue)

			floor := stoploss.Floor.Copy()
			observe(stoploss, floor.Sub(decimal.NewFromFloat64(0.30)).Float64(), false)

			So(stoploss.Status, ShouldEqual, types.ARMED)
			So(stoploss.TriggerReason, ShouldBeBlank)

			Convey("But a breach that holds should", func() {
				observe(stoploss, floor.Sub(decimal.NewFromFloat64(0.05)).Float64(), false)
				observe(stoploss, floor.Sub(decimal.NewFromFloat64(0.06)).Float64(), false)

				So(stoploss.Status, ShouldEqual, types.TRIGGERED)
				So(stoploss.TriggerReason, ShouldEqual, types.TriggerProtectedGiveback)

				Convey("And the exit it fires at should be net-positive after friction", func() {
					So(realized(stoploss.Floor), ShouldBeGreaterThan, 0)
				})
			})

			Convey("And a recovery in between should reset the confirmation", func() {
				observe(stoploss, 101.30, false)
				observe(stoploss, floor.Sub(decimal.NewFromFloat64(0.05)).Float64(), false)

				So(stoploss.Status, ShouldEqual, types.ARMED)
			})
		})
	})
}

func TestStoplossMonotonicity(t *testing.T) {
	Convey("Given a long path of adverse moves, recoveries and wicks", t, func() {
		stoploss, _ := fixtureEntry()

		path := []float64{
			99.70, 99.62 + 0.10, 99.85, 100.05, 99.90, 100.40, 100.15,
			101.00, 100.60, 101.40, 100.95, 101.80, 101.10, 102.20,
		}

		var (
			previousFloor *decimal.Decimal
			previousPeak  *decimal.Decimal
		)

		Convey("Neither the floor nor the peak should ever decrease", func() {
			for _, price := range path {
				observe(stoploss, price, false)

				if previousFloor != nil && stoploss.Floor != nil {
					So(stoploss.Floor.Cmp(previousFloor), ShouldBeGreaterThanOrEqualTo, 0)
				}

				if previousPeak != nil && stoploss.Peak != nil {
					So(stoploss.Peak.Cmp(previousPeak), ShouldBeGreaterThanOrEqualTo, 0)
				}

				if stoploss.Floor != nil {
					previousFloor = stoploss.Floor.Copy()
				}

				if stoploss.Peak != nil {
					previousPeak = stoploss.Peak.Copy()
				}
			}
		})
	})
}

func TestStoplossRetreatingQuote(t *testing.T) {
	Convey("Given a bid that steps up while its quantity is pulled", t, func() {
		stoploss, _ := fixtureEntry()

		observe(stoploss, 100.50, false)
		peak := stoploss.Peak.Copy()

		Convey("The hollow quote should not record a peak", func() {
			observe(stoploss, 101.50, true)

			So(stoploss.Peak.Cmp(peak), ShouldEqual, 0)
		})

		Convey("Ordinary cancellation should not suppress anything", func() {
			/*
				Some size is always being pulled in a working book. Gating on any
				positive value at all froze the trail permanently at entry, so
				the threshold is what separates a hollow quote from a live one.
			*/
			stoploss.Observe(types.StopEvidence{
				Symbol:         fixtureSymbol,
				ExecutableMark: decimal.NewFromFloat64(101.50),
				HollowReady:    true,
				HollowPressure: 0.05,
				ObservedAt:     time.Now().UTC(),
				Present:        true,
			})

			So(stoploss.Peak.Cmp(decimal.NewFromFloat64(101.50)), ShouldEqual, 0)
		})

		Convey("A stale reading should stop suppressing", func() {
			// A stalled analysis pipeline must not pin the trail forever on a
			// reading nobody has refreshed.
			stoploss.Observe(types.StopEvidence{
				Symbol:         fixtureSymbol,
				ExecutableMark: decimal.NewFromFloat64(101.50),
				HollowReady:    true,
				HollowPressure: 0.9,
				ObservedAt:     time.Now().UTC().Add(-time.Hour),
				Present:        true,
			})

			So(stoploss.Peak.Cmp(decimal.NewFromFloat64(101.50)), ShouldEqual, 0)
		})

		Convey("But a hollow quote should still be judged by the floors", func() {
			stoploss.Observe(types.StopEvidence{
				Symbol:         fixtureSymbol,
				ExecutableMark: stoploss.HardFloor.Sub(decimal.NewFromFloat64(0.01)),
				HollowReady:    true,
				HollowPressure: 0.9,
				ObservedAt:     time.Now().UTC(),
				Present:        true,
			})

			So(stoploss.Status, ShouldEqual, types.TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, types.TriggerHardRisk)
		})
	})
}

func TestStoplossTransitionAudit(t *testing.T) {
	Convey("Given a lot that arms protection and is then given back", t, func() {
		stoploss, plan := fixtureEntry()

		Convey("Construction should already have geometry to report", func() {
			opening := stoploss.DrainTransitions()

			So(opening, ShouldNotBeEmpty)
			So(opening[0].Seq, ShouldEqual, 1)

			reasons := make([]string, 0, len(opening))

			for _, transition := range opening {
				reasons = append(reasons, transition.Reason)
			}

			So(reasons, ShouldContain, "bound_on_fill")
		})

		Convey("Earlier transitions should not inherit a later trigger", func() {
			breach := stoploss.HardFloor.Sub(plan.TickSize)
			observe(stoploss, breach.Float64(), false)
			transitions := stoploss.DrainTransitions()
			var boundOnFill, hardRisk *types.StopTransition

			for index := range transitions {
				transition := &transitions[index]

				if transition.Reason == "bound_on_fill" {
					boundOnFill = transition
				}

				if transition.Reason == types.TriggerHardRisk {
					hardRisk = transition
				}
			}

			So(boundOnFill, ShouldNotBeNil)
			So(boundOnFill.Status, ShouldEqual, types.ARMED)
			So(boundOnFill.TriggerReason, ShouldBeBlank)
			So(hardRisk, ShouldNotBeNil)
			So(hardRisk.Status, ShouldEqual, types.TRIGGERED)
			So(hardRisk.TriggerReason, ShouldEqual, types.TriggerHardRisk)
			So(hardRisk.Mark.Cmp(hardRisk.HardFloor), ShouldEqual, -1)
		})

		Convey("Draining should hand each transition over exactly once", func() {
			So(stoploss.DrainTransitions(), ShouldNotBeEmpty)
			So(stoploss.DrainTransitions(), ShouldBeEmpty)

			observe(stoploss, 99.70, false)
			observe(stoploss, 101.20, false)

			armed := stoploss.DrainTransitions()
			So(armed, ShouldNotBeEmpty)
			So(stoploss.DrainTransitions(), ShouldBeEmpty)

			Convey("And the sequence should be strictly increasing across drains", func() {
				previous := uint64(0)

				for _, transition := range armed {
					So(transition.Seq, ShouldBeGreaterThan, previous)
					previous = transition.Seq
				}
			})

			Convey("And the arming and the exit should both be reported", func() {
				reasons := make([]string, 0)

				for _, transition := range armed {
					reasons = append(reasons, transition.Reason)
				}

				So(reasons, ShouldContain, "profit_armed")

				floor := stoploss.Floor.Copy()

				for range plan.ConfirmMarks {
					observe(stoploss, floor.Sub(decimal.NewFromFloat64(0.05)).Float64(), false)
				}

				So(stoploss.Status, ShouldEqual, types.TRIGGERED)

				exit := stoploss.DrainTransitions()
				exitReasons := make([]string, 0)

				for _, transition := range exit {
					exitReasons = append(exitReasons, transition.Reason)

					// Every row carries where the mark stood against the floor
					// it was judged by, which is what labels the outcome later.
					So(transition.Mark, ShouldNotBeNil)
					So(transition.Floor, ShouldNotBeNil)
				}

				So(exitReasons, ShouldContain, types.TriggerProtectedGiveback)
			})
		})
	})
}

func TestStoplossRebindFill(t *testing.T) {
	Convey("Given a stop armed against the ask the order was priced at", t, func() {
		plan := types.NewRiskPlan(types.RiskInputs{
			ReferencePrice: decimal.NewFromFloat64(100),
			Spread:         decimal.NewFromFloat64(0.10),
			Impact:         decimal.NewFromFloat64(0.02),
			TickSize:       decimal.NewFromFloat64(0.01),
			ExitFeeRate:    decimal.NewFromFloat64(0.0026),
			EntryFeeRate:   decimal.NewFromFloat64(0.0026),
			MaxLoss:        decimal.NewFromFloat64(5),
			Multiples:      types.DefaultRiskMultiples(),
		})

		stoploss := types.NewStoploss(
			context.Background(),
			fixtureSymbol,
			decimal.NewFromFloat64(100),
			decimal.NewFromFloat64(1),
			decimal.NewFromFloat64(0.26),
			decimal.NewFromFloat64(99.90),
			plan,
		)

		estimated := stoploss.ProfitLine.Copy()

		Convey("A fill worse than the estimate should raise the profit line", func() {
			stoploss.RebindFill(types.Fill{
				EntryPrice: decimal.NewFromFloat64(100.40),
				EntryFee:   decimal.NewFromFloat64(0.27),
				Qty:        decimal.NewFromFloat64(1),
			})

			So(stoploss.ProfitLine.Cmp(estimated), ShouldEqual, 1)
			So(stoploss.HardFloor.Cmp(decimal.NewFromFloat64(100.40)), ShouldEqual, -1)

			Convey("And the new line should still be net-positive on the realized basis", func() {
				proceeds := stoploss.ProfitLine.SetScale(12).
					Mul(decimal.NewFromFloat64(0.9974))

				So(proceeds.Sub(decimal.NewFromFloat64(100.67)).Sign(), ShouldBeGreaterThanOrEqualTo, 0)
			})
		})
	})
}

func BenchmarkStoplossObserve(b *testing.B) {
	stoploss, plan := fixtureEntry()
	mark := stoploss.ArmLine.Add(plan.TickSize).Float64()
	b.ResetTimer()

	for range b.N {
		observe(stoploss, mark, false)
		mark += plan.TickSize.Float64()
	}
}
