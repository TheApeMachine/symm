package logic

import (
	"math"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestReturnLadderPredict proves strict-prior direction learning on the fastest
rung: alternating one-epoch return processes must produce a ready ladder whose
newest prediction follows its own row without future leakage.
*/
func TestReturnLadderPredict(t *testing.T) {
	withResonanceRLS(t)

	Convey("Given alternating physical states with known next-return direction", t, func() {
		ladder, err := newReturnLadder()
		So(err, ShouldBeNil)
		midPrice := 100.0
		priorDirection := 0.0

		for index := range 96 {
			if index > 0 {
				midPrice *= math.Exp(0.001 * priorDirection)
			}

			direction := 1.0

			if index%2 == 1 {
				direction = -1
			}

			So(ladder.Advance(
				[]float64{direction, 0, 0, 0, 0},
				decimal.NewFromFloat64(midPrice),
			), ShouldBeNil)
			priorDirection = direction
		}

		Convey("Then the ladder is ready and the newest prediction is negative", func() {
			forecast := ladder.Forecast()

			So(forecast.Ready, ShouldBeTrue)
			So(forecast.HorizonEvents, ShouldEqual, uint64(1))
			So(forecast.ExpectedReturn, ShouldBeLessThan, 0)
			So(forecast.SkillLower, ShouldBeGreaterThan, 0)
			So(forecast.MeanMSE, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

/*
TestReturnLadderSelectsDeepHorizon proves the multi-resolution point of the
ladder: a drift whose per-epoch return is dominated by exactly alternating
noise is unpredictable one event ahead, but the noise cancels over two events,
so the two-epoch rung proves far stronger skill and selection must leave the
next-event horizon. This is the staircase blindness regression guard.
*/
func TestReturnLadderSelectsDeepHorizon(t *testing.T) {
	withResonanceRLS(t)

	Convey("Given drift hidden under exactly alternating one-epoch noise", t, func() {
		ladder, err := newReturnLadder()
		So(err, ShouldBeNil)
		midPrice := 100.0

		for index := range 256 {
			if index > 0 {
				noise := 0.01

				if index%2 == 0 {
					noise = -0.01
				}

				midPrice *= math.Exp(0.002 + noise)
			}

			So(ladder.Advance(
				[]float64{1, 0, 0, 0, 0},
				decimal.NewFromFloat64(midPrice),
			), ShouldBeNil)
		}

		Convey("Then selection proves skill beyond the next event", func() {
			forecast := ladder.Forecast()

			So(forecast.Ready, ShouldBeTrue)
			So(forecast.HorizonEvents, ShouldBeGreaterThanOrEqualTo, uint64(2))
			So(forecast.ExpectedReturn, ShouldBeGreaterThan, 0)
			So(forecast.SkillLower, ShouldBeGreaterThan, 0)
		})
	})
}

/*
BenchmarkReturnLadderAdvance measures one full ladder step: resolving every
due rung, teaching, and re-predicting all horizons for the new row.
*/
func BenchmarkReturnLadderAdvance(b *testing.B) {
	withResonanceRLS(b)
	ladder, err := newReturnLadder()

	if err != nil {
		b.Fatal(err)
	}

	midPrice := 100.0
	features := []float64{1, 0, 0, 0, 0}
	b.ReportAllocs()

	for b.Loop() {
		midPrice *= math.Exp(0.0001)

		if err := ladder.Advance(
			features, decimal.NewFromFloat64(midPrice),
		); err != nil {
			b.Fatal(err)
		}
	}
}
