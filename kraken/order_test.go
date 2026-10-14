package kraken_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestAddOrderRequestMeasurementRoundTrip(t *testing.T) {
	Convey("Given a valid kraken.AddOrderRequest", t, func() {
		original := &kraken.AddOrderRequest{
			Pair:    "BTC/USD",
			Type:    "buy",
			OrdType: "market",
			Volume:  "1.25",
			ClOrdId: "order-xyz-123",
		}

		Convey("ToMeasurement serializes order without raw shortcuts", func() {
			meas := original.ToMeasurement(123456789)
			So(meas, ShouldNotBeNil)
			So(meas.Label, ShouldEqual, "BTC/USD")
			So(meas.Source, ShouldEqual, "orders")
			So(meas.Raw(), ShouldBeNil)

			Convey("and NewAddOrderRequestFromMeasurement restores all fields", func() {
				restored, err := kraken.NewAddOrderRequestFromMeasurement(meas)
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
