package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSlippagePriceUsesHalfSpread(t *testing.T) {
	convey.Convey("Given bid/ask around last", t, func() {
		fill := SlippagePrice(100, 99, 101, "buy", 0)

		convey.Convey("It should price a buy above mid", func() {
			convey.So(fill, convey.ShouldEqual, 101)
		})
	})
}
