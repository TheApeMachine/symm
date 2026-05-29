package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestMeasurementFromEngine(t *testing.T) {
	Convey("Given a pumpdump coiled compression reading", t, func() {
		reading := engine.Measurement{
			Source:     "pumpdump",
			Category:   engine.CatCoiledCompression,
			Confidence: 0.7,
		}

		measurement, err := MeasurementFromEngine(reading)

		Convey("It should map to the tree category type", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, perspectives.CategoryCoiledCompression)
			So(measurement.HasCategory(perspectives.CategoryCoiledCompression), ShouldBeTrue)
		})
	})

	Convey("Given a reading with no category", t, func() {
		_, err := MeasurementFromEngine(engine.Measurement{Source: "fluid"})

		Convey("It should error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
