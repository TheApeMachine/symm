package order

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestOrderFillTerminal(t *testing.T) {
	convey.Convey("Given partial cumulative quantity", t, func() {
		convey.Convey("It should not be terminal", func() {
			convey.So(OrderFillTerminal(Fill{CumQty: 0.5, OrderQty: 1}), convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given full cumulative quantity", t, func() {
		convey.Convey("It should be terminal", func() {
			convey.So(OrderFillTerminal(Fill{CumQty: 1, OrderQty: 1}), convey.ShouldBeTrue)
		})
	})
}
