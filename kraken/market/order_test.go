package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOrderSetEnvelopeType(t *testing.T) {
	Convey("Given an order event", t, func() {
		order := Order{Symbol: "BTC/EUR"}

		order.SetEnvelopeType(OrderSnapshot)

		Convey("It should mark snapshot envelopes", func() {
			So(order.IsSnapshot(), ShouldBeTrue)
		})
	})
}

func TestOrderAvailableWithoutToken(t *testing.T) {
	Convey("Given no order token source", t, func() {
		SetOrderTokenSource(nil)

		Convey("It should report order channel unavailable", func() {
			So(OrderAvailable(), ShouldBeFalse)
		})
	})
}
