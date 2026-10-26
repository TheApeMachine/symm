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
			So(val.Epoch, ShouldEqual, uint64(123456789))
		})
	})
}
