package types

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRiskPlanRefresh(t *testing.T) {
	Convey("Given a present plan and a wider live execution-noise reading", t, func() {
		plan := NewRiskPlan(RiskInputs{
			ReferencePrice: riskDecimal(t, "100"),
			Spread:         riskDecimal(t, "0.02"),
			Impact:         riskDecimal(t, "0.01"),
			TickSize:       riskDecimal(t, "0.01"),
		})
		riskDistance := plan.RiskDistance
		entryNoiseBand := plan.EntryNoiseBand
		armBuffer := plan.ArmBuffer
		lockBuffer := plan.LockBuffer

		refreshed := plan.Refresh(
			riskDecimal(t, "0.06"),
			riskDecimal(t, "0.02"),
		)

		Convey("It should widen only the live noise and trail geometry", func() {
			So(refreshed.NoiseBand.Cmp(riskDecimal(t, "0.08")), ShouldEqual, 0)
			So(refreshed.TrailDistance.Cmp(riskDecimal(t, "0.16")), ShouldEqual, 0)
			So(refreshed.RiskDistance.Cmp(riskDistance), ShouldEqual, 0)
			So(refreshed.EntryNoiseBand.Cmp(entryNoiseBand), ShouldEqual, 0)
			So(refreshed.ArmBuffer.Cmp(armBuffer), ShouldEqual, 0)
			So(refreshed.LockBuffer.Cmp(lockBuffer), ShouldEqual, 0)
		})
	})

	Convey("Given a live noise reading below the instrument minimum", t, func() {
		plan := NewRiskPlan(RiskInputs{
			ReferencePrice: riskDecimal(t, "100"),
			Spread:         riskDecimal(t, "0.04"),
			TickSize:       riskDecimal(t, "0.01"),
		})

		Convey("It should leave the admitted plan unchanged", func() {
			refreshed := plan.Refresh(riskDecimal(t, "0.01"), nil)

			So(refreshed.NoiseBand.Cmp(plan.NoiseBand), ShouldEqual, 0)
			So(refreshed.TrailDistance.Cmp(plan.TrailDistance), ShouldEqual, 0)
		})
	})
}

func TestNewRiskPlan(t *testing.T) {
	Convey("Given no authoritative instrument tick", t, func() {
		Convey("It should refuse to invent venue precision", func() {
			plan := NewRiskPlan(RiskInputs{
				ReferencePrice: riskDecimal(t, "64951.1"),
				Spread:         riskDecimal(t, "0.1"),
			})

			So(plan.Present, ShouldBeFalse)
		})
	})

	Convey("Given a high-priced instrument with an eight-decimal venue tick", t, func() {
		plan := NewRiskPlan(RiskInputs{
			ReferencePrice: riskDecimal(t, "64951.10000001"),
			Spread:         riskDecimal(t, "0.00000001"),
			Impact:         riskDecimal(t, "0.00000001"),
			TickSize:       riskDecimal(t, "0.00000001"),
		})

		Convey("It should retain the instrument lattice and every minimum tick", func() {
			So(plan.Present, ShouldBeTrue)
			So(plan.TickSize.Cmp(riskDecimal(t, "0.00000001")), ShouldEqual, 0)
			So(plan.NoiseBand.Cmp(riskDecimal(t, "0.00000004")), ShouldEqual, 0)
			So(plan.RiskDistance.Cmp(riskDecimal(t, "0.00000012")), ShouldEqual, 0)
			So(plan.TrailDistance.Cmp(riskDecimal(t, "0.00000008")), ShouldEqual, 0)
			So(plan.ArmBuffer.Cmp(plan.LockBuffer), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a risk distance that consumes the executable price", t, func() {
		Convey("It should reject a non-positive hard-floor geometry", func() {
			plan := NewRiskPlan(RiskInputs{
				ReferencePrice: riskDecimal(t, "1.00"),
				Spread:         riskDecimal(t, "0.50"),
				TickSize:       riskDecimal(t, "0.01"),
			})

			So(plan.Present, ShouldBeFalse)
		})
	})
}

func TestRiskPlanLossPerUnit(t *testing.T) {
	Convey("Given a tick-rounded floor and taker fees on both crossings", t, func() {
		plan := RiskPlan{
			Present:      true,
			RiskDistance: riskDecimal(t, "0.014"),
			TickSize:     riskDecimal(t, "0.01"),
			EntryFeeRate: riskDecimal(t, "0.0025"),
			ExitFeeRate:  riskDecimal(t, "0.0025"),
		}

		Convey("It should include the adverse tick projection and both fees", func() {
			loss := plan.LossPerUnit(riskDecimal(t, "100.005"))

			So(loss, ShouldNotBeNil)
			So(loss.Cmp(riskDecimal(t, "0.5149875")), ShouldEqual, 0)
			So(loss.Cmp(riskDecimal(t, "0.014")), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given an absent risk contract", t, func() {
		Convey("It should not state a per-unit loss", func() {
			So((RiskPlan{}).LossPerUnit(riskDecimal(t, "100")), ShouldBeNil)
		})
	})
}

func TestRiskPlanMaxQuantity(t *testing.T) {
	Convey("Given a fee-inclusive per-unit loss and a fixed loss budget", t, func() {
		plan := RiskPlan{
			Present:      true,
			RiskDistance: riskDecimal(t, "0.014"),
			TickSize:     riskDecimal(t, "0.01"),
			EntryFeeRate: riskDecimal(t, "0.0025"),
			ExitFeeRate:  riskDecimal(t, "0.0025"),
			MaxLoss:      riskDecimal(t, "10"),
		}
		lossPerUnit := plan.LossPerUnit(riskDecimal(t, "100.005"))

		Convey("It should cap the lot at the budget rather than the bare price distance", func() {
			quantity := plan.MaxQuantity(riskDecimal(t, "100.005"))
			realizedBudget := quantity.Mul(lossPerUnit)

			So(quantity, ShouldNotBeNil)
			So(quantity.Cmp(riskDecimal(t, "20")), ShouldBeLessThan, 0)
			So(realizedBudget.Cmp(plan.MaxLoss), ShouldBeLessThanOrEqualTo, 0)
			So(plan.MaxLoss.Sub(realizedBudget).Cmp(
				riskDecimal(t, "0.00000001"),
			), ShouldBeLessThan, 0)
		})
	})
}

func TestCeilToTick(t *testing.T) {
	Convey("Given prices on and between authoritative instrument ticks", t, func() {
		cases := []struct {
			name     string
			price    string
			tick     string
			expected string
		}{
			{name: "exact cent boundary", price: "100.01", tick: "0.01", expected: "100.01"},
			{name: "half cent above boundary", price: "100.005", tick: "0.01", expected: "100.01"},
			{name: "one sub-tick quantum above", price: "64951.100000009", tick: "0.00000001", expected: "64951.10000001"},
			{name: "exact eight-decimal boundary", price: "64951.10000001", tick: "0.00000001", expected: "64951.10000001"},
			{name: "coarse high-price instrument", price: "987654321", tick: "10000", expected: "987660000"},
		}

		for _, testCase := range cases {
			Convey(testCase.name, func() {
				actual := ceilToTick(
					riskDecimal(t, testCase.price),
					riskDecimal(t, testCase.tick),
				)

				So(actual.Cmp(riskDecimal(t, testCase.expected)), ShouldEqual, 0)
			})
		}
	})

	Convey("Given no usable instrument tick", t, func() {
		price := riskDecimal(t, "100.005")

		Convey("It should return the unprojected price", func() {
			So(ceilToTick(price, nil), ShouldEqual, price)
			So(ceilToTick(price, decimal.NewFromInt64(0)), ShouldEqual, price)
		})
	})
}

func TestFloorToTick(t *testing.T) {
	Convey("Given prices on and between authoritative instrument ticks", t, func() {
		cases := []struct {
			name     string
			price    string
			tick     string
			expected string
		}{
			{name: "exact cent boundary", price: "100.01", tick: "0.01", expected: "100.01"},
			{name: "half cent above boundary", price: "100.005", tick: "0.01", expected: "100.00"},
			{name: "one sub-tick quantum below", price: "64951.100000009", tick: "0.00000001", expected: "64951.10000000"},
			{name: "exact eight-decimal boundary", price: "64951.10000001", tick: "0.00000001", expected: "64951.10000001"},
			{name: "coarse high-price instrument", price: "987654321", tick: "10000", expected: "987650000"},
		}

		for _, testCase := range cases {
			Convey(testCase.name, func() {
				actual := floorToTick(
					riskDecimal(t, testCase.price),
					riskDecimal(t, testCase.tick),
				)

				So(actual.Cmp(riskDecimal(t, testCase.expected)), ShouldEqual, 0)
			})
		}
	})

	Convey("Given no usable instrument tick", t, func() {
		price := riskDecimal(t, "100.005")

		Convey("It should return the unprojected price", func() {
			So(floorToTick(price, nil), ShouldEqual, price)
			So(floorToTick(price, decimal.NewFromInt64(0)), ShouldEqual, price)
		})
	})
}

func riskDecimal(testingTB testing.TB, value string) *decimal.Decimal {
	testingTB.Helper()
	amount, err := decimal.NewFromString(value)

	if err != nil {
		testingTB.Fatalf("risk decimal %q: %v", value, err)
	}

	return amount
}
