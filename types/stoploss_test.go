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

		Convey("It should lock profit immediately at the arming line", func() {
			stoploss.Update(stoploss.ArmAt)

			So(stoploss.Locked, ShouldBeTrue)
			So(stoploss.Floor.Cmp(stoploss.LockFloor), ShouldEqual, 0)
			So(stoploss.Floor.Cmp(stoploss.ProfitLine), ShouldBeGreaterThan, 0)
		})

		Convey("It should ratchet upward after profit lock", func() {
			stoploss.Update(stoploss.ArmAt)
			stoploss.Update(decimal.NewFromFloat64(110))

			So(stoploss.Floor.Cmp(decimal.NewFromFloat64(108)), ShouldEqual, 0)
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

func BenchmarkStoplossUpdate(b *testing.B) {
	stoploss := stoplossFixture(b)
	stoploss.Update(stoploss.ArmAt)
	mark := decimal.NewFromFloat64(110)
	b.ResetTimer()

	for b.Loop() {
		stoploss.Update(mark)
	}
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
		forecast,
		RiskPlan{
			Present:       true,
			RiskDistance:  decimal.NewFromFloat64(2),
			TrailDistance: decimal.NewFromFloat64(2),
			ArmBuffer:     decimal.NewFromFloat64(2),
			LockBuffer:    decimal.NewFromFloat64(1),
			MinEdge:       decimal.NewFromFloat64(1),
			ExitFeeRate:   zeroRate,
			EntryFeeRate:  zeroRate,
			TickSize:      decimal.NewFromFloat64(0.01),
		},
	)

	if err != nil {
		testingTB.Fatalf("stoploss: %v", err)
	}

	return stoploss
}
