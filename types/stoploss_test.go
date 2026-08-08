package types

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
)

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
		forecast, err := NewResonanceForecast(
			[]float64{-0.0001},
			[]float64{1},
			1,
			0.9,
		)
		So(err, ShouldBeNil)

		zeroRate := decimal.NewFromInt64(0)
		stoploss, err := NewStoploss(
			context.Background(),
			"SOL/USD",
			decimal.NewFromFloat64(73.72),
			decimal.NewFromFloat64(73.70),
			forecast,
			decimal.NewFromFloat64(0.01),
			zeroRate,
			zeroRate,
		)
		So(err, ShouldBeNil)

		Convey("It should apply predicted drawdown to the sellable mark", func() {
			So(stoploss.Floor.Cmp(decimal.NewFromFloat64(73.69)), ShouldEqual, 0)

			stoploss.Update(decimal.NewFromFloat64(73.70))

			So(stoploss.Status, ShouldEqual, ARMED)
		})
	})
}

func TestStoplossRebindFill(t *testing.T) {
	Convey("Given an admitted stop with remembered forecast reach", t, func() {
		stoploss := stoplossFixture(t)
		floor := stoploss.Floor
		trailDistance := stoploss.trailDistance

		Convey("It should update fill economics without rebuilding reach", func() {
			err := stoploss.RebindFill(decimal.NewFromFloat64(100.05))

			So(err, ShouldBeNil)
			So(stoploss.Floor.Cmp(floor), ShouldEqual, 0)
			So(stoploss.trailDistance.Cmp(trailDistance), ShouldEqual, 0)
			So(stoploss.ProfitLine.Cmp(decimal.NewFromFloat64(100.05)), ShouldEqual, 0)
		})
	})
}

func TestStoplossUpdate(t *testing.T) {
	Convey("Given a forecast-aware stop before profit lock", t, func() {
		stoploss := stoplossFixture(t)
		preLockFloor := stoploss.Floor

		Convey("It should survive the deepest expected intermediate dip", func() {
			stoploss.Update(preLockFloor)

			So(stoploss.Status, ShouldEqual, ARMED)
			So(stoploss.Floor.Cmp(preLockFloor), ShouldEqual, 0)
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

		Convey("It should ratchet upward after profit lock", func() {
			stoploss.Update(stoploss.ArmAt)
			stoploss.Update(decimal.NewFromFloat64(110))
			expected := floorToTick(
				decimal.NewFromFloat64(110).SetScale(riskScale).Sub(
					stoploss.trailDistance,
				),
				stoploss.tickSize,
			)

			So(stoploss.Floor.Cmp(expected), ShouldEqual, 0)
		})

		Convey("It should never lower a ratcheted floor", func() {
			stoploss.Update(stoploss.ArmAt)
			stoploss.Update(decimal.NewFromFloat64(110))
			ratcheted := stoploss.Floor
			stoploss.Update(decimal.NewFromFloat64(109))

			So(stoploss.Floor.Cmp(ratcheted), ShouldEqual, 0)
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
}

type testReporter interface {
	Helper()
	Fatalf(string, ...any)
}

func stoplossFixture(testingTB testReporter) *Stoploss {
	testingTB.Helper()
	forecast, err := NewResonanceForecast(
		[]float64{-0.01, -0.02, 0.06},
		[]float64{1, 0.5, 0.5},
		3,
		0.9,
	)

	if err != nil {
		testingTB.Fatalf("forecast: %v", err)
	}

	zeroRate := decimal.NewFromInt64(0)
	stoploss, err := NewStoploss(
		context.Background(),
		"SIM/USD",
		decimal.NewFromFloat64(100),
		decimal.NewFromFloat64(100),
		forecast,
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
	forecast, err := NewResonanceForecast(
		[]float64{-0.01, -0.02, 0.06},
		[]float64{1, 0.5, 0.5},
		3,
		0.9,
	)

	if err != nil {
		b.Fatalf("forecast: %v", err)
	}

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
