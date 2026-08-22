package types

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
)

func testForecast(value float64) *learning.RLSOutput {
	return &learning.RLSOutput{Value: value, Ready: true, Scale: 0.01, DegreesOfFreedom: 1}
}

func TestNewStoploss(t *testing.T) {
	Convey("Given a trusted path with a temporary retained drawdown", t, func() {
		stoploss := stoplossFixture(t)
		expected := decimal.NewFromFloat64(100 * math.Exp(-0.02))

		Convey("It should set the pre-lock floor from the deepest path point", func() {
			So(stoploss.Floor.Cmp(expected), ShouldBeLessThanOrEqualTo, 0)
			So(expected.Sub(stoploss.Floor).Cmp(
				decimal.NewFromFloat64(0.01),
			), ShouldBeLessThan, 0)
			So(stoploss.Floor.Cmp(decimal.NewFromFloat64(98)), ShouldNotEqual, 0)
			So(stoploss.Locked, ShouldBeFalse)
		})
	})

	Convey("Given an entry ask above the executable mark", t, func() {
		forecast := testForecast(-0.0001)

		zeroRate := decimal.NewFromInt64(0)
		stoploss, err := NewStoploss(
			context.Background(),
			"SOL/USD",
			decimal.NewFromFloat64(73.72),
			decimal.NewFromFloat64(73.70),
			forecast,
			nil,
			decimal.NewFromFloat64(0.01),
			zeroRate,
			zeroRate,
		)
		So(err, ShouldBeNil)

		Convey("It should retain enough room to recover the entry crossing", func() {
			So(stoploss.Floor.Cmp(decimal.NewFromFloat64(73.68)), ShouldEqual, 0)

			stoploss.Update(decimal.NewFromFloat64(73.69))

			So(stoploss.Status, ShouldEqual, ARMED)
		})
	})

	Convey("Given a direction lean and no priced return path", t, func() {
		forecast := testForecast(0.8)
		zeroRate := decimal.NewFromInt64(0)

		Convey("It should ignore the lean magnitude and keep the one-tick lattice", func() {
			stoploss, err := NewStoploss(
				context.Background(),
				"BTC/USD",
				decimal.NewFromFloat64(100.02),
				decimal.NewFromFloat64(100),
				forecast,
				nil,
				decimal.NewFromFloat64(0.01),
				zeroRate,
				zeroRate,
			)

			So(err, ShouldBeNil)
			So(stoploss.Floor.Cmp(decimal.NewFromFloat64(99.98)), ShouldEqual, 0)
		})
	})

	Convey("Given a forecast path that never falls below its starting price", t, func() {
		forecast := testForecast(0.01)
		zeroRate := decimal.NewFromInt64(0)

		Convey("It should put the floor beyond the measured entry spread", func() {
			stoploss, err := NewStoploss(
				context.Background(),
				"BTC/USD",
				decimal.NewFromFloat64(100.02),
				decimal.NewFromFloat64(100),
				forecast,
				nil,
				decimal.NewFromFloat64(0.01),
				zeroRate,
				zeroRate,
			)

			So(err, ShouldBeNil)
			So(stoploss.Floor.Cmp(decimal.NewFromFloat64(99.98)), ShouldEqual, 0)
			So(stoploss.Floor.Cmp(stoploss.Peak), ShouldBeLessThan, 0)
		})
	})

	Convey("Given a downside smaller than one executable tick", t, func() {
		forecast := testForecast(-1e-8)
		zeroRate := decimal.NewFromInt64(0)

		Convey("It should use tick rounding while preserving strict floor separation", func() {
			stoploss, err := NewStoploss(
				context.Background(),
				"BTC/USD",
				decimal.NewFromFloat64(100.02),
				decimal.NewFromFloat64(100),
				forecast,
				nil,
				decimal.NewFromFloat64(0.01),
				zeroRate,
				zeroRate,
			)

			So(err, ShouldBeNil)
			So(stoploss.Floor.Cmp(stoploss.Peak), ShouldBeLessThan, 0)
			So(scaled(stoploss.Peak).Sub(stoploss.Floor).Cmp(
				decimal.NewFromFloat64(0.02),
			), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})

	Convey("Given the MEZO entry spread and taker fees", t, func() {
		forecast := testForecast(0.01)
		entryPrice, err := decimal.NewFromString("0.00985")
		So(err, ShouldBeNil)
		mark, err := decimal.NewFromString("0.00984")
		So(err, ShouldBeNil)
		tick, err := decimal.NewFromString("0.00001")
		So(err, ShouldBeNil)
		feeRate, err := decimal.NewFromString("0.0026")
		So(err, ShouldBeNil)
		stoploss, err := NewStoploss(
			context.Background(),
			"MEZO/USD",
			entryPrice,
			mark,
			forecast,
			nil,
			tick,
			feeRate,
			feeRate,
		)
		So(err, ShouldBeNil)
		profitLine, err := decimal.NewFromString("0.00991")
		So(err, ShouldBeNil)
		floor, err := decimal.NewFromString("0.00977")
		So(err, ShouldBeNil)
		oneTickLower, err := decimal.NewFromString("0.00982")
		So(err, ShouldBeNil)

		Convey("It should not treat a one-tick move as exit evidence", func() {
			So(stoploss.ProfitLine.Cmp(profitLine), ShouldEqual, 0)
			So(stoploss.Floor.Cmp(floor), ShouldEqual, 0)

			stoploss.Update(oneTickLower)

			So(stoploss.Status, ShouldEqual, ARMED)
		})
	})

	Convey("Given a positive cumulative forecast whose path first draws down", t, func() {
		forecast := testForecast(0.03)
		forwardCurve := []float64{-0.02, 0.01, 0.04}
		zeroRate := decimal.NewFromInt64(0)
		stoploss, err := NewStoploss(
			context.Background(),
			"BTC/USD",
			decimal.NewFromFloat64(100),
			decimal.NewFromFloat64(100),
			forecast,
			forwardCurve,
			decimal.NewFromFloat64(0.01),
			zeroRate,
			zeroRate,
		)
		So(err, ShouldBeNil)

		Convey("It should survive above the deepest path point and fire at the boundary", func() {
			expected := decimal.NewFromFloat64(100 * math.Exp(-0.02))
			So(stoploss.Floor.Cmp(expected), ShouldBeLessThanOrEqualTo, 0)
			So(expected.Sub(stoploss.Floor).Cmp(
				decimal.NewFromFloat64(0.01),
			), ShouldBeLessThan, 0)

			stoploss.Update(stoploss.Floor.Add(decimal.NewFromFloat64(0.01)))
			So(stoploss.Status, ShouldEqual, ARMED)

			stoploss.Update(stoploss.Floor)
			So(stoploss.Status, ShouldEqual, TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, TriggerHardFloor)
		})
	})

	Convey("Given a positive path with no predicted drawdown", t, func() {
		forecast := testForecast(0.03)
		forwardCurve := []float64{0.01, 0.01, 0.01}
		zeroRate := decimal.NewFromInt64(0)
		stoploss, err := NewStoploss(
			context.Background(),
			"BTC/USD",
			decimal.NewFromFloat64(100.02),
			decimal.NewFromFloat64(100),
			forecast,
			forwardCurve,
			decimal.NewFromFloat64(0.01),
			zeroRate,
			zeroRate,
		)

		Convey("It should keep the executable one-tick lattice boundary", func() {
			So(err, ShouldBeNil)
			So(stoploss.Floor.Cmp(decimal.NewFromFloat64(99.98)), ShouldEqual, 0)
		})
	})
}

func TestStoplossRebindFill(t *testing.T) {
	Convey("Given an admitted stop with remembered forecast reach", t, func() {
		stoploss := stoplossFixture(t)
		floor := stoploss.Floor
		trailDistance := stoploss.TrailDistance

		Convey("It should update fill economics without narrowing forecast reach", func() {
			err := stoploss.RebindFill(
				decimal.NewFromFloat64(100.05),
				decimal.NewFromFloat64(100),
			)

			So(err, ShouldBeNil)
			So(stoploss.Floor.Cmp(floor), ShouldEqual, 0)
			So(stoploss.TrailDistance.Cmp(trailDistance), ShouldEqual, 0)
			So(stoploss.ProfitLine.Cmp(decimal.NewFromFloat64(100.05)), ShouldEqual, 0)
		})
	})
}

func TestStoplossUpdate(t *testing.T) {
	Convey("Given a forecast-aware stop before profit lock", t, func() {
		stoploss := stoplossFixture(t)
		preLockFloor := stoploss.Floor

		Convey("It should fire on the first mark at the hard floor", func() {
			stoploss.Update(preLockFloor)

			So(stoploss.Status, ShouldEqual, TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, TriggerHardFloor)
			So(stoploss.TriggerMark.Cmp(preLockFloor), ShouldEqual, 0)
		})

		Convey("It should leave the forecast jitter room before profit lock", func() {
			stoploss.Update(stoploss.ArmAt.Sub(
				decimal.NewFromFloat64(0.01),
			))

			So(stoploss.Locked, ShouldBeFalse)
			So(stoploss.Floor.Cmp(preLockFloor), ShouldEqual, 0)
		})

		Convey("It should lock profit immediately at the arming line", func() {
			stoploss.Update(stoploss.ArmAt)

			So(stoploss.Locked, ShouldBeTrue)
			So(stoploss.Floor.Cmp(stoploss.LockFloor), ShouldEqual, 0)
			So(stoploss.Floor.Cmp(stoploss.ProfitLine), ShouldBeGreaterThan, 0)
		})

		Convey("It should fire immediately at the protected floor", func() {
			stoploss.Update(stoploss.ArmAt)
			protected := stoploss.Floor

			stoploss.Update(protected)

			So(stoploss.Status, ShouldEqual, TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, TriggerProtectedFloor)
		})

		Convey("It should start trailing once profit clears the learned giveback tolerance", func() {
			stoploss.ArmAt = stoploss.ArmAt.Add(decimal.NewFromFloat64(10))

			stoploss.Update(decimal.NewFromFloat64(100.5))

			Convey("A single step does not yet lock, because its own scale is the giveback room", func() {
				So(stoploss.Locked, ShouldBeFalse)
			})

			stoploss.Update(decimal.NewFromFloat64(101.0))
			stoploss.Update(decimal.NewFromFloat64(101.5))
			stoploss.Update(decimal.NewFromFloat64(102.0))
			stoploss.Update(decimal.NewFromFloat64(102.5))

			Convey("A repeated step scale lets the candidate floor clear profit", func() {
				So(stoploss.Locked, ShouldBeTrue)
				So(stoploss.Floor.Cmp(stoploss.ProfitLine), ShouldBeGreaterThan, 0)
			})
		})

		Convey("It should hold a surge through breathing and exit when the burst unwinds a central band", func() {
			stoploss.TrailDistance = decimal.NewFromFloat64(0.10)
			stoploss.NoiseBand = decimal.NewFromFloat64(0.01)

			stoploss.Update(decimal.NewFromFloat64(100.5))
			So(stoploss.SurgeArmed, ShouldBeTrue)
			So(stoploss.Status, ShouldEqual, ARMED)
			So(stoploss.MomentumFloor, ShouldNotBeNil)

			stoploss.Update(decimal.NewFromFloat64(100.4))

			So(stoploss.Status, ShouldEqual, ARMED)

			stoploss.Update(decimal.NewFromFloat64(99.99))

			So(stoploss.Status, ShouldEqual, TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, TriggerPumpMomentumLost)
		})

		Convey("It should survive a multi-leg run with pullbacks proportional to the run", func() {
			// Under an entry-frozen trail this sequence exits on the second
			// leg: the burst peak places the floor one entry-scale trail below
			// itself and the 101.5 breathing mark falls through it.
			stoploss.TrailDistance = decimal.NewFromFloat64(0.10)
			stoploss.NoiseBand = decimal.NewFromFloat64(0.01)

			stoploss.Update(decimal.NewFromFloat64(102))
			stoploss.Update(decimal.NewFromFloat64(101.5))
			stoploss.Update(decimal.NewFromFloat64(103))
			stoploss.Update(decimal.NewFromFloat64(104))
			stoploss.Update(decimal.NewFromFloat64(103))
			stoploss.Update(decimal.NewFromFloat64(106))
			stoploss.Update(decimal.NewFromFloat64(104.5))

			So(stoploss.Status, ShouldEqual, ARMED)
			So(stoploss.TriggerReason, ShouldEqual, "")
			So(stoploss.Peak.Cmp(decimal.NewFromFloat64(106)), ShouldEqual, 0)
			So(stoploss.Floor.Cmp(decimal.NewFromFloat64(104.5)), ShouldBeLessThan, 0)
		})

		Convey("It should ratchet upward after profit lock", func() {
			stoploss.Update(stoploss.ArmAt)
			stoploss.Update(decimal.NewFromFloat64(104))
			expected := floorToTick(
				scaled(decimal.NewFromFloat64(104)).Sub(
					largest(
						stoploss.TrailDistance,
						decimal.NewFromFloat64(stoploss.learnedMoveBoundary()),
					),
				),
				stoploss.TickSize,
			)

			So(stoploss.Floor.Cmp(expected), ShouldEqual, 0)
			So(stoploss.Floor.Cmp(stoploss.ProfitLine), ShouldBeGreaterThan, 0)
		})

		Convey("It should never lower a ratcheted floor", func() {
			stoploss.Update(stoploss.ArmAt)
			stoploss.Update(decimal.NewFromFloat64(110))
			ratcheted := stoploss.Floor
			stoploss.Update(decimal.NewFromFloat64(109))

			So(stoploss.Floor.Cmp(ratcheted), ShouldEqual, 0)
		})

		Convey("It should hold through consolidation flags during a parabolic run without stagnation triggers", func() {
			stoploss.Update(stoploss.ArmAt)
			stoploss.Update(decimal.NewFromFloat64(120))
			So(stoploss.isParabolicRun(), ShouldBeTrue)

			stoploss.Update(decimal.NewFromFloat64(118.5))
			stoploss.Update(decimal.NewFromFloat64(118.0))
			stoploss.Update(decimal.NewFromFloat64(118.2))
			stoploss.Update(decimal.NewFromFloat64(118.1))
			stoploss.Update(decimal.NewFromFloat64(118.3))

			So(stoploss.Status, ShouldEqual, ARMED)
			So(stoploss.TriggerReason, ShouldEqual, "")
		})

		Convey("It should raise a lagged lock floor back to the lock line", func() {
			stoploss.Update(stoploss.ArmAt)
			stoploss.Floor = preLockFloor
			stoploss.Update(stoploss.ArmAt.Add(decimal.NewFromFloat64(0.01)))

			So(stoploss.Locked, ShouldBeTrue)
			So(stoploss.Floor.Cmp(stoploss.LockFloor), ShouldBeGreaterThanOrEqualTo, 0)
			So(stoploss.Status, ShouldEqual, ARMED)
		})

		Convey("It should hold when a profitable mark repeats at the same price", func() {
			stoploss.Update(stoploss.ArmAt)
			stoploss.Update(stoploss.ArmAt)

			So(stoploss.Status, ShouldEqual, ARMED)
		})

		Convey("It should take profit when distinct marks drift beyond the run's central band", func() {
			// The trail is set wider than the learned step scale, so the
			// confirmed stagnation path is the tighter boundary here, the way
			// a real RiskPlan separates the two regimes.
			stoploss.TrailDistance = decimal.NewFromFloat64(0.50)
			stoploss.NoiseBand = decimal.NewFromFloat64(0.01)

			for _, price := range []float64{100.1, 100.2, 100.3, 100.4, 100.5, 100.6} {
				stoploss.Update(decimal.NewFromFloat64(price))
			}

			stoploss.Update(decimal.NewFromFloat64(100.50))
			stoploss.Update(decimal.NewFromFloat64(100.50))
			stoploss.Update(decimal.NewFromFloat64(100.49))
			stoploss.Update(decimal.NewFromFloat64(100.48))

			So(stoploss.Status, ShouldEqual, TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, TriggerProfitStagnation)
		})

		Convey("It should hold oscillation whose giveback is inside the run's central band", func() {
			stoploss.TrailDistance = decimal.NewFromFloat64(0.50)
			stoploss.NoiseBand = decimal.NewFromFloat64(0.01)

			stoploss.Update(decimal.NewFromFloat64(100.5))
			stoploss.Update(decimal.NewFromFloat64(100.45))
			stoploss.Update(decimal.NewFromFloat64(100.44))
			stoploss.Update(decimal.NewFromFloat64(100.43))

			So(stoploss.Status, ShouldEqual, ARMED)
			So(stoploss.TriggerReason, ShouldEqual, "")
		})

		Convey("It should hold an unprofitable mark that is not a new high", func() {
			stoploss.Update(stoploss.ProfitLine.Sub(decimal.NewFromFloat64(0.01)))
			stoploss.Update(stoploss.ProfitLine.Sub(decimal.NewFromFloat64(0.02)))

			So(stoploss.Status, ShouldEqual, ARMED)
		})
	})
}

func TestStoplossArmClock(t *testing.T) {
	Convey("Given a stop that has already begun counting marks", t, func() {
		stoploss := underwaterStoploss(t, 3)
		stoploss.ArmClock()
		stoploss.Update(decimal.NewFromFloat64(100))

		Convey("It should ignore a second arm so pre-fill marks cannot be erased", func() {
			stoploss.ArmClock()
			So(stoploss.Observed, ShouldEqual, 1)
			So(stoploss.ClockArmed, ShouldBeTrue)
		})
	})
}

func TestStoplossReconsider(t *testing.T) {
	Convey("Given a filled lot whose admitted path has not elapsed", t, func() {
		stoploss := underwaterStoploss(t, 3)
		stoploss.ArmClock()
		stoploss.Update(decimal.NewFromFloat64(100))
		stoploss.Update(decimal.NewFromFloat64(99.99))

		Convey("It should keep the lot without consulting future-return economics", func() {
			stoploss.Reconsider(-1, 1)
			So(stoploss.Status, ShouldEqual, ARMED)
			So(stoploss.Observed, ShouldEqual, 2)
		})
	})

	Convey("Given a consumed transition horizon while the lot is still red", t, func() {
		stoploss := underwaterStoploss(t, 2)
		observeHorizon(stoploss)

		Convey("It should release the slot regardless of forecast or utility arguments", func() {
			stoploss.Reconsider(10, -10)
			So(stoploss.Status, ShouldEqual, TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, TriggerHorizonExpired)
			So(stoploss.TriggerMark.Cmp(stoploss.Mark), ShouldEqual, 0)
		})
	})

	Convey("Given a consumed horizon while the mark is already through the profit line", t, func() {
		stoploss := underwaterStoploss(t, 2)
		stoploss.ArmClock()
		stoploss.Update(stoploss.ProfitLine)
		stoploss.Update(stoploss.ProfitLine)

		Convey("It should leave profitable regulation on Update", func() {
			So(stoploss.Observed, ShouldEqual, 2)
			stoploss.Reconsider(0, 0)
			So(stoploss.Status, ShouldEqual, ARMED)
		})
	})

	Convey("Given a locked lot after its horizon has been consumed", t, func() {
		stoploss := underwaterStoploss(t, 1)
		stoploss.ArmClock()
		stoploss.Update(stoploss.ArmAt)

		Convey("It should not steal the profit-lock path", func() {
			stoploss.Reconsider(0, 0)
			So(stoploss.Locked, ShouldBeTrue)
			So(stoploss.Status, ShouldEqual, ARMED)
		})
	})

	Convey("Given a clock that was never armed", t, func() {
		stoploss := underwaterStoploss(t, 1)
		stoploss.Update(decimal.NewFromFloat64(100))
		stoploss.Update(decimal.NewFromFloat64(100))

		Convey("It should not count pre-fill marks as path consumption", func() {
			stoploss.Reconsider(0, 0)
			So(stoploss.Status, ShouldEqual, ARMED)
			So(stoploss.Observed, ShouldEqual, 0)
		})
	})

	Convey("Given an admitted path with no forecast steps", t, func() {
		zeroRate := decimal.NewFromInt64(0)
		stoploss, err := NewStoploss(
			context.Background(),
			"SIM/USD",
			decimal.NewFromFloat64(100),
			decimal.NewFromFloat64(100),
			testForecast(-0.02),
			nil,
			decimal.NewFromFloat64(0.01),
			zeroRate,
			zeroRate,
		)
		So(err, ShouldBeNil)
		stoploss.ArmClock()

		// The mark must stay above the one-tick floor this nil-curve lot
		// carries: a floor breach is a hard exit, not horizon consumption.
		stoploss.Update(decimal.NewFromFloat64(99.995))

		Convey("It should refuse to invent a transition horizon", func() {
			stoploss.Reconsider(0, 0)
			So(stoploss.Status, ShouldEqual, ARMED)
			So(stoploss.Horizon, ShouldEqual, 0)
		})
	})

	Convey("Given an elapsed horizon reached over successive observations", t, func() {
		stoploss := underwaterStoploss(t, 3)
		stoploss.ArmClock()
		stoploss.Update(decimal.NewFromFloat64(100))
		stoploss.Reconsider(0, 0)
		So(stoploss.Status, ShouldEqual, ARMED)
		stoploss.Update(decimal.NewFromFloat64(99.99))
		stoploss.Reconsider(0, 0)
		So(stoploss.Status, ShouldEqual, ARMED)

		// The third mark must stay above the hard floor: a lot that breaches
		// its floor has already exited and no longer consumes horizon.
		stoploss.Update(decimal.NewFromFloat64(99.985))

		Convey("It should fire only once the admitted path has actually elapsed", func() {
			stoploss.Reconsider(0, 0)
			So(stoploss.Status, ShouldEqual, TRIGGERED)
			So(stoploss.Observed, ShouldEqual, 3)
		})
	})
}

func TestStoplossTriggerStrategyExit(t *testing.T) {
	Convey("Given an armed lot whose continuation value turned negative", t, func() {
		stoploss := underwaterStoploss(t, 1)

		Convey("It should trigger through the regulator's strategy seam", func() {
			So(stoploss.TriggerStrategyExit(), ShouldBeNil)
			So(stoploss.Status, ShouldEqual, TRIGGERED)
			So(stoploss.TriggerReason, ShouldEqual, TriggerContinuationEV)
			So(stoploss.TriggerMark, ShouldNotBeNil)
			So(stoploss.TriggerMark.Cmp(stoploss.Mark), ShouldEqual, 0)
		})
	})

	Convey("Given an already triggered lot", t, func() {
		stoploss := underwaterStoploss(t, 1)

		So(stoploss.TriggerStrategyExit(), ShouldBeNil)

		Convey("It should refuse a second strategy trigger", func() {
			So(stoploss.TriggerStrategyExit(), ShouldNotBeNil)
		})
	})

	Convey("Given a locked, advancing lot", t, func() {
		stoploss := underwaterStoploss(t, 1)
		stoploss.Locked = true

		Convey("It should refuse to yank the winner ahead of its trail", func() {
			So(stoploss.TriggerStrategyExit(), ShouldNotBeNil)
			So(stoploss.Status, ShouldEqual, ARMED)
		})
	})
}

func TestRestoreStoploss(t *testing.T) {
	Convey("Given a stored stoploss after its floor ratchets", t, func() {
		original := stoplossFixture(t)
		original.Update(original.ArmAt)
		original.Update(decimal.NewFromFloat64(110))
		state, err := original.MarshalState()
		So(err, ShouldBeNil)

		Convey("It should continue from the exact stored floor", func() {
			restored, err := RestoreStoploss(context.Background(), state)
			So(err, ShouldBeNil)
			So(restored.Floor.Cmp(original.Floor), ShouldEqual, 0)
			So(restored.Peak.Cmp(original.Peak), ShouldEqual, 0)

			nextMark := decimal.NewFromFloat64(111)
			original.Update(nextMark)
			restored.Update(nextMark)
			So(restored.Floor.Cmp(original.Floor), ShouldEqual, 0)
		})
	})

	Convey("Given a stored stop armed by an unusual peak acceleration", t, func() {
		original := stoplossFixture(t)
		original.TrailDistance = decimal.NewFromFloat64(0.10)
		original.NoiseBand = decimal.NewFromFloat64(0.01)
		original.Update(decimal.NewFromFloat64(102))
		So(original.SurgeArmed, ShouldBeTrue)
		state, err := original.MarshalState()
		So(err, ShouldBeNil)

		Convey("It should preserve the momentum exhaustion boundary", func() {
			restored, restoreErr := RestoreStoploss(context.Background(), state)
			So(restoreErr, ShouldBeNil)
			So(restored.SurgeArmed, ShouldBeTrue)
			So(restored.SurgeMove.Cmp(original.SurgeMove), ShouldEqual, 0)
			So(restored.MomentumFloor.Cmp(original.MomentumFloor), ShouldEqual, 0)

			restored.Update(decimal.NewFromFloat64(102.01))
			So(restored.Status, ShouldEqual, ARMED)

			restored.Update(decimal.NewFromFloat64(99.9))
			So(restored.Status, ShouldEqual, TRIGGERED)
		})
	})

	Convey("Given a stored stop whose surge latch lost its momentum geometry", t, func() {
		original := stoplossFixture(t)
		encoded, err := original.MarshalState()
		So(err, ShouldBeNil)
		state := Stoploss{}
		So(json.Unmarshal(encoded, &state), ShouldBeNil)
		state.SurgeArmed = true
		state.SurgeMove = nil
		state.MomentumFloor = nil
		encoded, err = json.Marshal(state)
		So(err, ShouldBeNil)

		Convey("It should reject an unsafe half-restored exhaustion detector", func() {
			restored, restoreErr := RestoreStoploss(context.Background(), encoded)
			So(restoreErr, ShouldNotBeNil)
			So(restored, ShouldBeNil)
		})
	})

	Convey("Given a legacy stored stop whose floor collapsed onto its peak", t, func() {
		original := stoplossFixture(t)
		encoded, err := original.MarshalState()
		So(err, ShouldBeNil)
		state := Stoploss{}
		So(json.Unmarshal(encoded, &state), ShouldBeNil)
		state.Floor = state.Peak
		encoded, err = json.Marshal(state)
		So(err, ShouldBeNil)

		Convey("It should refuse to recover unsafe geometry", func() {
			restored, err := RestoreStoploss(context.Background(), encoded)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "floor must remain below peak")
			So(restored, ShouldBeNil)
		})
	})

	Convey("Given a stored stop whose stay clock had already advanced", t, func() {
		original := underwaterStoploss(t, 3)
		observeHorizon(original)
		state, err := original.MarshalState()
		So(err, ShouldBeNil)

		Convey("It should resume the same remaining path length", func() {
			restored, err := RestoreStoploss(context.Background(), state)
			So(err, ShouldBeNil)
			So(restored.Horizon, ShouldEqual, 3)
			So(restored.Observed, ShouldEqual, 3)
			So(restored.ClockArmed, ShouldBeTrue)
			restored.Reconsider(0, 0)
			So(restored.Status, ShouldEqual, TRIGGERED)
		})
	})
}

func TestStoplossAccessors(t *testing.T) {
	Convey("Given an initialized stoploss with horizon and fee rates", t, func() {
		stoploss := underwaterStoploss(t, 5)
		fee := decimal.NewFromFloat64(0.0026)
		stoploss.EntryFeeRate = fee
		stoploss.ExitFeeRate = fee

		Convey("It should report zero observed before arming", func() {
			So(stoploss.Observed, ShouldEqual, 0)
			So(stoploss.Horizon, ShouldEqual, 5)
			So(stoploss.Maturing(), ShouldBeFalse)
			So(stoploss.EntryFeeRate.Cmp(fee), ShouldEqual, 0)
			So(stoploss.ExitFeeRate.Cmp(fee), ShouldEqual, 0)
		})

		Convey("After arming the clock and advancing marks within horizon", func() {
			stoploss.ArmClock()
			stoploss.Update(decimal.NewFromFloat64(100))
			stoploss.Update(decimal.NewFromFloat64(100))

			So(stoploss.Observed, ShouldEqual, 2)
			So(stoploss.Maturing(), ShouldBeTrue)
		})

		Convey("Once the horizon is reached", func() {
			observeHorizon(stoploss)

			So(stoploss.Observed, ShouldEqual, 5)
			So(stoploss.Maturing(), ShouldBeFalse)
		})
	})
}

type testReporter interface {
	Helper()
	Fatalf(string, ...any)
}

func underwaterStoploss(testingTB testReporter, horizon int) *Stoploss {
	testingTB.Helper()
	curve := make([]float64, horizon)

	for index := range curve {
		curve[index] = 0.01
	}

	zeroRate := decimal.NewFromInt64(0)
	stoploss, err := NewStoploss(
		context.Background(),
		"BTC/USD",
		decimal.NewFromFloat64(100.02),
		decimal.NewFromFloat64(100),
		testForecast(0.03),
		curve,
		decimal.NewFromFloat64(0.01),
		zeroRate,
		zeroRate,
	)

	if err != nil {
		testingTB.Fatalf("stoploss: %v", err)
	}

	return stoploss
}

func observeHorizon(stoploss *Stoploss) {
	stoploss.ArmClock()

	for range stoploss.Horizon {
		stoploss.Update(decimal.NewFromFloat64(100))
	}
}

func stoplossFixture(testingTB testReporter) *Stoploss {
	testingTB.Helper()
	forecast := testForecast(-0.02)

	zeroRate := decimal.NewFromInt64(0)
	stoploss, err := NewStoploss(
		context.Background(),
		"SIM/USD",
		decimal.NewFromFloat64(100),
		decimal.NewFromFloat64(100),
		forecast,
		[]float64{-0.02},
		decimal.NewFromFloat64(0.01),
		zeroRate,
		zeroRate,
	)

	if err != nil {
		testingTB.Fatalf("stoploss: %v", err)
	}

	return stoploss
}

func BenchmarkNewStoploss(b *testing.B) {
	forecast := testForecast(-0.02)

	zeroRate := decimal.NewFromInt64(0)
	entry := decimal.NewFromFloat64(100.02)
	mark := decimal.NewFromFloat64(100)
	tick := decimal.NewFromFloat64(0.01)
	b.ResetTimer()

	for b.Loop() {
		stoploss, err := NewStoploss(
			context.Background(),
			"SIM/USD",
			entry,
			mark,
			forecast,
			nil,
			tick,
			zeroRate,
			zeroRate,
		)

		if err != nil {
			b.Fatalf("stoploss: %v", err)
		}

		if err := stoploss.Close(); err != nil {
			b.Fatalf("close stoploss: %v", err)
		}
	}
}

func BenchmarkStoplossUpdate(b *testing.B) {
	stoploss := stoplossFixture(b)
	stoploss.Update(stoploss.ArmAt)
	mark := decimal.NewFromFloat64(110)
	b.ResetTimer()

	for b.Loop() {
		stoploss.Update(mark)
	}
}

func BenchmarkStoplossReconsider(b *testing.B) {
	stoploss := underwaterStoploss(b, 2)
	observeHorizon(stoploss)
	b.ResetTimer()

	for b.Loop() {
		stoploss.Status = ARMED
		stoploss.Reconsider(0.01, 0)
	}
}
