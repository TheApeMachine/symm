package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementToResonanceFiltersTaskInputs(t *testing.T) {
	Convey("Given a measurement containing market facts and direction evidence", t, func() {
		price := 100.0
		drive := 0.8
		separation := 0.4
		measurement := &Measurement{
			Source: SourceCVD,
			Symbol: "BTC/USD",
			Tick:   7,
			Metadata: map[string]float64{
				"bid": 99,
				"ask": 101,
			},
			Metrics: map[string]MetricSample{
				MetricKey(MetricTradePrice, SideNone): {
					Raw: price, Normalized: &price,
				},
				MetricKey(MetricDrive, SideNone): {
					Raw: drive, Normalized: &drive,
				},
				MetricKey(MetricHypothesisSeparation, SideNone): {
					Raw: separation, Normalized: &separation,
				},
			},
		}

		result := MeasurementToResonance("BTC/USD", measurement)

		Convey("It should retain the mark and conditioned evidence only", func() {
			So(result, ShouldNotBeNil)
			So(result.Mark, ShouldEqual, 100.0)
			So(result.Readings, ShouldHaveLength, 2)
			_, hasPrice := result.Readings["cvd:BTC/USD:trade_price"]
			So(hasPrice, ShouldBeFalse)
			_, hasDrive := result.Readings["cvd:BTC/USD:drive"]
			So(hasDrive, ShouldBeTrue)
		})
	})
}
