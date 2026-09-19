package cvd_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/cvd"
)

func TestCVDPipeline(t *testing.T) {
	Convey("Given CVD zero-struct Value closures", t, func() {
		assemble := cvd.NewAssemble()
		flow := cvd.NewFlow()
		buyQty := cvd.NewBuyQty()
		cvdMetric := cvd.NewCVD()

		Convey("Nil input produces nil fill", func() {
			fill := assemble(nil)
			So(fill, ShouldBeNil)
		})

		Convey("A buy trade calculates correct CVD and BuyQty", func() {
			trade := map[string]any{
				"trade": map[string]any{
					"data": map[string]any{
						"side":      "buy",
						"symbol":    "BTC/USD",
						"price":     50000.0,
						"qty":       1.5,
						"timestamp": int64(1700000000),
					},
				},
			}

			fill := assemble(trade)
			So(fill, ShouldNotBeNil)
			So(fill.Side, ShouldEqual, "buy")
			So(fill.Qty, ShouldEqual, 1.5)

			reading := flow(fill)
			So(buyQty(reading), ShouldEqual, 1.5)
			So(cvdMetric(reading), ShouldEqual, 1.5)
		})
	})
}
