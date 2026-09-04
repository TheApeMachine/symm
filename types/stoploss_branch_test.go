package types

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBranchResolve(t *testing.T) {
	Convey("Given the Stoploss decision tree", t, func() {
		Convey("a breached unlocked floor resolves to the hard-floor exit", func() {
			stoploss := &Stoploss{
				Status: ARMED,
				Floor:  decimal.NewFromInt64(100),
			}
			mark := decimal.NewFromInt64(100)

			resolved := stoplossBranches.Resolve(stoploss, nil, mark)

			So(resolved, ShouldBeTrue)
			So(stoploss.TriggerReason, ShouldEqual, TriggerHardFloor)
		})

		Convey("a breached lock floor resolves to the protected-floor child", func() {
			stoploss := &Stoploss{
				Status:    ARMED,
				Locked:    true,
				Floor:     decimal.NewFromInt64(100),
				LockFloor: decimal.NewFromInt64(100),
			}
			mark := decimal.NewFromInt64(100)

			resolved := stoplossBranches.Resolve(stoploss, nil, mark)

			So(resolved, ShouldBeTrue)
			So(stoploss.TriggerReason, ShouldEqual, TriggerProtectedFloor)
		})

		Convey("a breached raised floor resolves to the trailing-floor child", func() {
			stoploss := &Stoploss{
				Status:    ARMED,
				Locked:    true,
				Floor:     decimal.NewFromInt64(105),
				LockFloor: decimal.NewFromInt64(100),
			}
			mark := decimal.NewFromInt64(104)

			resolved := stoplossBranches.Resolve(stoploss, nil, mark)

			So(resolved, ShouldBeTrue)
			So(stoploss.TriggerReason, ShouldEqual, TriggerTrailingFloor)
		})

		Convey("an invalid previously observed book resolves to regime invalidation", func() {
			stoploss := &Stoploss{
				Status:       ARMED,
				BookObserved: true,
				Mark:         decimal.NewFromInt64(101),
			}
			surface := &ExecutionSurface{BookComplete: false}

			resolved := stoplossBranches.Resolve(stoploss, surface, nil)

			So(resolved, ShouldBeTrue)
			So(stoploss.TriggerReason, ShouldEqual, TriggerRegimeInvalidated)
			So(stoploss.TriggerMark.Cmp(stoploss.Mark), ShouldEqual, 0)
		})

		Convey("a locked floor without quantity coverage resolves to regime invalidation", func() {
			stoploss := &Stoploss{Status: ARMED, Locked: true}
			surface := &ExecutionSurface{
				SellableQty:      decimal.NewFromInt64(10),
				ExecutableVWAP:   decimal.NewFromInt64(101),
				FloorCoverageQty: decimal.NewFromInt64(9),
				BookComplete:     true,
				FullyExecutable:  true,
			}

			resolved := stoplossBranches.Resolve(stoploss, surface, nil)

			So(resolved, ShouldBeTrue)
			So(stoploss.TriggerReason, ShouldEqual, TriggerRegimeInvalidated)
			So(stoploss.TriggerMark.Cmp(surface.ExecutableVWAP), ShouldEqual, 0)
		})

		Convey("an armed surge unwinding through its line resolves to momentum exit", func() {
			stoploss := &Stoploss{
				Status:        ARMED,
				SurgeArmed:    true,
				Peak:          decimal.NewFromInt64(105),
				MomentumFloor: decimal.NewFromInt64(2),
				TickSize:      decimal.NewFromFloat64(0.01),
			}
			mark := decimal.NewFromInt64(103)

			resolved := stoplossBranches.Resolve(stoploss, nil, mark)

			So(resolved, ShouldBeTrue)
			So(stoploss.TriggerReason, ShouldEqual, TriggerPumpMomentumLost)
		})

		Convey("a confirmed profitable drift resolves to stagnation exit", func() {
			stoploss := &Stoploss{
				Status:               ARMED,
				ProfitLatched:        true,
				ProfitLine:           decimal.NewFromInt64(100),
				Peak:                 decimal.NewFromInt64(102),
				NoiseBand:            decimal.NewFromFloat64(0.10),
				ConfirmMarks:         3,
				DistinctNonPeakMarks: 3,
			}
			mark := decimal.NewFromInt64(101)

			resolved := stoplossBranches.Resolve(stoploss, nil, mark)

			So(resolved, ShouldBeTrue)
			So(stoploss.TriggerReason, ShouldEqual, TriggerProfitStagnation)
		})

		Convey("a mark outside every terminal condition remains armed", func() {
			stoploss := &Stoploss{
				Status: ARMED,
				Floor:  decimal.NewFromInt64(90),
			}

			resolved := stoplossBranches.Resolve(
				stoploss,
				nil,
				decimal.NewFromInt64(100),
			)

			So(resolved, ShouldBeFalse)
			So(stoploss.Status, ShouldEqual, ARMED)
		})

		Convey("a reflexive momentum cascade suppresses regime invalidation and lets position breathe", func() {
			stoploss := &Stoploss{
				Status:       ARMED,
				BookObserved: true,
				Locked:       true,
				Causative: CausativeContext{
					HawkesBranchingRatio: 0.92,
					OIGrowthVelocity:     12.5,
				},
			}
			surface := &ExecutionSurface{
				BookComplete:     false,
				SellableQty:      decimal.NewFromInt64(10),
				FloorCoverageQty: decimal.NewFromInt64(5),
			}

			resolved := stoplossBranches.Resolve(stoploss, surface, nil)

			So(resolved, ShouldBeFalse)
			So(stoploss.Status, ShouldEqual, ARMED)
		})

		Convey("an active liquidity sweep protects a breached floor from stoploss hunting", func() {
			stoploss := &Stoploss{
				Status: ARMED,
				Floor:  decimal.NewFromInt64(100),
				Causative: CausativeContext{
					NetReplenishmentBid: 0.75,
					ActivePerspectives: map[string]string{
						"pullback": "LiquiditySweep",
					},
				},
			}
			mark := decimal.NewFromInt64(99)

			resolved := stoplossBranches.Resolve(stoploss, nil, mark)

			So(resolved, ShouldBeFalse)
			So(stoploss.Status, ShouldEqual, ARMED)
		})

		Convey("a fast vertical ignition pump exit triggers when momentum stalls", func() {
			stoploss := &Stoploss{
				Status: ARMED,
				Causative: CausativeContext{
					ActivePerspectives: map[string]string{
						"momentum": "Stalling",
					},
				},
			}
			mark := decimal.NewFromInt64(150)

			resolved := stoplossBranches.Resolve(stoploss, nil, mark)

			So(resolved, ShouldBeTrue)
			So(stoploss.TriggerReason, ShouldEqual, TriggerPumpMomentumLost)
		})
	})
}

func BenchmarkBranchResolve(b *testing.B) {
	stoploss := &Stoploss{
		Status: ARMED,
		Floor:  decimal.NewFromInt64(90),
	}
	mark := decimal.NewFromInt64(100)
	b.ReportAllocs()

	for b.Loop() {
		stoplossBranches.Resolve(stoploss, nil, mark)
	}
}
