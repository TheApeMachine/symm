package logic

import (
	"encoding/json"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementUnlessPublishable(t *testing.T) {
	Convey("Given a complete measurement", t, func() {
		measurement := Measurement{
			Source:     SourceFluid,
			Symbol:     "BTC/USD",
			Price:      100,
			Strength:   0.5,
			Confidence: 0.8,
			Surprise:   0.2,
		}

		Convey("It should remain publishable", func() {
			published := measurement.UnlessPublishable()
			So(published.Source, ShouldEqual, SourceFluid)

			_, err := json.Marshal(published)
			So(err, ShouldBeNil)
		})
	})

	Convey("Given a measurement with NaN confidence", t, func() {
		measurement := Measurement{
			Source:     SourceCausal,
			Symbol:     "BTC/USD",
			Price:      100,
			Strength:   0.5,
			Confidence: math.NaN(),
			Surprise:   0.2,
		}

		Convey("It should be withheld", func() {
			So(measurement.UnlessPublishable().Source, ShouldEqual, SourceType(""))
		})
	})
}
