package kraken_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestAddOrderRequestValueRoundTrip(t *testing.T) {
	Convey("Given a valid kraken.AddOrderRequest", t, func() {
		original := &kraken.AddOrderRequest{
			Pair:    "BTC/USD",
			Type:    "buy",
			OrdType: "market",
			Volume:  "1.25",
			ClOrdId: "order-xyz-123",
		}

		Convey("ToValue serializes order into Value", func() {
			val := original.ToValue(123456789)
			So(val, ShouldNotBeNil)
			So(val.Scope, ShouldEqual, "BTC/USD")
			So(val.Role, ShouldEqual, "order")

			Convey("and NewAddOrderRequestFromValue restores all fields", func() {
				restored, err := kraken.NewAddOrderRequestFromValue(val)
				So(err, ShouldBeNil)
				So(restored, ShouldNotBeNil)
				So(restored.Pair, ShouldEqual, original.Pair)
				So(restored.Type, ShouldEqual, original.Type)
				So(restored.OrdType, ShouldEqual, original.OrdType)
				So(restored.Volume, ShouldEqual, original.Volume)
				So(restored.ClOrdId, ShouldEqual, original.ClOrdId)
			})
		})
	})
}
