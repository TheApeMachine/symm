package kraken_test

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestAddOrderRequestJSONRoundTrip(t *testing.T) {
	Convey("Given a valid kraken.AddOrderRequest", t, func() {
		original := &kraken.AddOrderRequest{
			Pair:    "BTC/USD",
			Type:    "buy",
			OrdType: "market",
			Volume:  "1.25",
			ClOrdId: "order-xyz-123",
		}

		Convey("sonic.Marshal round-trips the order", func() {
			raw, err := sonic.Marshal(original)
			So(err, ShouldBeNil)

			var decoded kraken.AddOrderRequest
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)
			So(decoded.Pair, ShouldEqual, "BTC/USD")
			So(decoded.Type, ShouldEqual, "buy")
			So(decoded.OrdType, ShouldEqual, "market")
			So(decoded.Volume, ShouldEqual, "1.25")
			So(decoded.ClOrdId, ShouldEqual, "order-xyz-123")
		})
	})
}
