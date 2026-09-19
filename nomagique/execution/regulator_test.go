package execution

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRegulator(t *testing.T) {
	Convey("Given a new Regulator for an asset", t, func() {
		regulator := NewRegulator("ASSET/USD")

		Convey("Initial state is zero", func() {
			initial := regulator(nil)
			So(initial, ShouldNotBeNil)
			So(initial.Quantity.Sign(), ShouldEqual, 0)
			So(initial.Basis.Sign(), ShouldEqual, 0)
			So(initial.Realized.Sign(), ShouldEqual, 0)
		})

		Convey("When processing a buy fill", func() {
			state := regulator(&Fill{
				OrderID: "order-1",
				Side:    "buy",
				CumQty:  decimal.NewFromFloat64(2.0),
				CumCost: decimal.NewFromFloat64(100.0),
				Fee:     decimal.NewFromFloat64(0.5),
				Status:  "filled",
			})

			So(state, ShouldNotBeNil)
			So(state.Quantity.Cmp(decimal.NewFromFloat64(2.0)), ShouldEqual, 0)
			So(state.Basis.Cmp(decimal.NewFromFloat64(100.0)), ShouldEqual, 0)
			So(state.EntryFee.Cmp(decimal.NewFromFloat64(0.5)), ShouldEqual, 0)

			Convey("When partially selling inventory", func() {
				sellCost, _ := decimal.NewFromString("60.00")
				sellFee, _ := decimal.NewFromString("0.20")
				sellState := regulator(&Fill{
					OrderID: "order-2",
					Side:    "sell",
					CumQty:  decimal.NewFromFloat64(1.0),
					CumCost: sellCost,
					Fee:     sellFee,
					Status:  "filled",
				})

				So(sellState, ShouldNotBeNil)
				// Remaining quantity should be 1.0
				So(sellState.Quantity.Cmp(decimal.NewFromFloat64(1.0)), ShouldEqual, 0)
				// Remaining basis should be 50.0 (half of 100)
				So(sellState.Basis.Cmp(decimal.NewFromFloat64(50.0)), ShouldEqual, 0)
				// Realized: costDelta (60) - feeDelta (0.2) - allocBasis (50) - allocFee (0.25) = 9.55
				expectedRealized, _ := decimal.NewFromString("9.55")
				t.Logf("sellState.Realized = %s, expected = %s", sellState.Realized.String(), expectedRealized.String())
				So(sellState.Realized.Cmp(expectedRealized), ShouldEqual, 0)
			})
		})
	})
}
