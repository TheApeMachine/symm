package types

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
)

/*
TestNewHoldingSeedsPendingStoploss verifies construction leaves inventory pending
with a live regulator attached before any fill arrives.
*/
func TestNewHoldingSeedsPendingStoploss(t *testing.T) {
	Convey("Given NewHolding for a sized lot", t, func() {
		holding := NewHolding(
			context.Background(),
			"BTC/USD",
			decimal.NewFromFloat64(0.25),
		)

		So(holding.Symbol, ShouldEqual, "BTC/USD")
		So(holding.Qty.Float64(), ShouldEqual, 0.25)
		So(holding.Status, ShouldEqual, PENDING)
		So(holding.Stoploss, ShouldNotBeNil)
		So(holding.IsOpportunity, ShouldBeFalse)
	})
}

/*
TestHoldingValidateAcceptsFractionalQty verifies errnie.Validate accepts a
sub-unit lot so trade does not reject sized crypto entries as invalid.
*/
func TestHoldingValidateAcceptsFractionalQty(t *testing.T) {
	Convey("Given a holding sized below one base unit", t, func() {
		holding := Holding{
			Symbol: "BTC/USD",
			Qty:    decimal.NewFromFloat64(0.001),
		}

		Convey("When Validate runs", func() {
			err := errnie.Validate(&holding)

			Convey("Then fractional positive qty is accepted", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}

/*
TestHoldingValidateRejectsMissingQty verifies required qty fails when unset.
*/
func TestHoldingValidateRejectsMissingQty(t *testing.T) {
	Convey("Given a holding without qty", t, func() {
		So(errnie.Validate(&Holding{Symbol: "BTC/USD"}), ShouldNotBeNil)
	})
}
