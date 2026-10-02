package data_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestRecurrence(t *testing.T) {
	Convey("Recurrence measures nearest distance and percentile across history", t, func() {
		node := data.NewRecurrence("z1", "z2")

		meas1 := data.NewMeasurement[float64]("test")
		meas1.WriteMetric("z1", 1.0)
		meas1.WriteMetric("z2", 1.0)

		for range node.Next(transport.NewOne(unsafe.Pointer(&meas1)).Next(nil)) {
		}

		_, hasDist1 := meas1.LookupMetric("historical_path_distance")
		So(hasDist1, ShouldBeFalse)

		meas2 := data.NewMeasurement[float64]("test")
		meas2.WriteMetric("z1", 1.1)
		meas2.WriteMetric("z2", 1.1)

		for range node.Next(transport.NewOne(unsafe.Pointer(&meas2)).Next(nil)) {
		}

		dist2, hasDist2 := meas2.LookupMetric("historical_path_distance")
		So(hasDist2, ShouldBeTrue)
		So(dist2.Raw, ShouldBeGreaterThan, 0)

		perc2, hasPerc2 := meas2.LookupMetric("historical_path_percentile")
		So(hasPerc2, ShouldBeTrue)
		So(perc2.Raw, ShouldEqual, 1.0)
	})
}
