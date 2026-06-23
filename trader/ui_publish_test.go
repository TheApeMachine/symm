package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestCollapseMeasurementsForUI(testingTB *testing.T) {
	Convey("Given measurements for many scopes", testingTB, func() {
		hawkesAnchor := datura.Acquire("test", datura.APPJSON)
		hawkesAnchor.MergeOutput("confidence", 0.8)
		hawkesAnchor.SetOrigin("hawkes")
		hawkesAnchor.WithScope("BTC/USD")

		hawkesAlt := datura.Acquire("test", datura.APPJSON)
		hawkesAlt.MergeOutput("confidence", 0.2)
		hawkesAlt.SetOrigin("hawkes")
		hawkesAlt.WithScope("ETH/USD")

		fluid := datura.Acquire("test", datura.APPJSON)
		fluid.Merge("calibrating", true)
		fluid.SetOrigin("fluid")
		fluid.WithScope("BTC/USD")

		grouped := map[string][]*datura.Artifact{
			"BTC/USD": {hawkesAnchor},
			"ETH/USD": {hawkesAlt},
		}
		calibrating := []*datura.Artifact{fluid}

		uiCalibrating, uiGrouped := collapseMeasurementsForUI(
			"BTC/USD",
			calibrating,
			grouped,
		)

		Convey("It should keep one reading per origin preferring anchor scope", func() {
			So(len(uiCalibrating), ShouldEqual, 1)
			So(len(uiGrouped), ShouldEqual, 1)
			So(len(uiGrouped["BTC/USD"]), ShouldEqual, 1)

			origin, err := uiGrouped["BTC/USD"][0].Origin()

			So(err, ShouldBeNil)
			So(origin, ShouldEqual, "hawkes")
			So(datura.Peek[float64](uiGrouped["BTC/USD"][0], "output", "confidence"), ShouldEqual, 0.8)
		})
	})
}
