package audit

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func storedDetection(class string, b, c float64) *data.Measurement {
	return data.NewMeasurement(1, "BTC/USD", "detector", 1, 1, &data.StringEntry{Key: "type", Value: class}).Write(
		data.NewMetric("b_price", b, data.UnitPrice, data.TimescaleTick),
		data.NewMetric("c_price", c, data.UnitPrice, data.TimescaleTick),
	)
}

func TestFeeProvenance(t *testing.T) {
	Convey("Given detections made at a 0.8% taker fee", t, func() {
		// Break-even fee of a 2% rise is 0.0099; of a 1.5% rise 0.00744.
		detections := []*data.Measurement{
			storedDetection("up", 100, 102),
			storedDetection("down", 102, 100),
			storedDetection("up_friction", 100, 101.5),
		}

		Convey("declaring 0.8% is consistent", func() {
			So(checkFeeProvenance(detections, 0.008).Consistent, ShouldBeTrue)
		})

		Convey("declaring 0.26% contradicts the near misses", func() {
			So(checkFeeProvenance(detections, 0.0026).Consistent, ShouldBeFalse)
		})

		Convey("declaring 1.5% contradicts the cleared legs", func() {
			So(checkFeeProvenance(detections, 0.015).Consistent, ShouldBeFalse)
		})

		Convey("no bounding detections establish nothing", func() {
			So(checkFeeProvenance(nil, 0.008).Consistent, ShouldBeFalse)
		})
	})
}
