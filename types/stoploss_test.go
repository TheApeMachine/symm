package types_test

import (
	"context"
	"testing"

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
		Uncertainty:    0.002,
		TickSize:       decimal.NewFromFloat64(0.01),
		ExitFeeRate:    decimal.NewFromFloat64(0.0026),
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
func observe(stoploss *types.Stoploss, price float64, retreating bool) {
	evidence := types.StopEvidence{
		Symbol:         fixtureSymbol,
		ExecutableMark: decimal.NewFromFloat64(price),
		Present:        true,
	}

	if retreating {
		evidence.RetreatReady = true
		evidence.RetreatPressure = 0.4
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
			capacity := plan.MaxQuantity()
			So(capacity, ShouldNotBeNil)

			loss := plan.RiskDistance.SetScale(12).Mul(capacity)
			So(loss.Cmp(plan.MaxLoss), ShouldBeLessThanOrEqualTo, 0)
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

		Convey("The retreating quote should not record a peak", func() {
			observe(stoploss, 101.50, true)

			So(stoploss.Peak.Cmp(peak), ShouldEqual, 0)
		})

		Convey("But it should still be judged by the floors", func() {
			stoploss.Observe(types.StopEvidence{
				Symbol:          fixtureSymbol,
				ExecutableMark:  stoploss.HardFloor.Sub(decimal.NewFromFloat64(0.01)),
				RetreatReady:    true,
				RetreatPressure: 0.9,
				Present:         true,
			})

			So(stoploss.Status, ShouldEqual, types.TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, types.TriggerHardRisk)
		})
	})
}

func TestStoplossRebindFill(t *testing.T) {
	Convey("Given a stop armed against the ask the order was priced at", t, func() {
		plan := types.NewRiskPlan(types.RiskInputs{
			ReferencePrice: decimal.NewFromFloat64(100),
			Spread:         decimal.NewFromFloat64(0.10),
			Impact:         decimal.NewFromFloat64(0.02),
			Uncertainty:    0.002,
			TickSize:       decimal.NewFromFloat64(0.01),
			ExitFeeRate:    decimal.NewFromFloat64(0.0026),
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
