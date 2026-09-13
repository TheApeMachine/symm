package sentiment

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

var schema = new(Ticker).Register().Metrics

/*
tick builds the measurement the workload's data management would hand the
signal: the register's declared schema with the feed's last price written.
*/
func tick(symbol string, price float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("sentiment", schema)
	m.Label, m.At, m.From = symbol, at, at
	m.Metrics["last"] = m.Metrics["last"].Write(price)

	return m
}

func timestamp(second int64) time.Time {
	return time.Unix(1_700_000_000+second, 0)
}

func TestTickerStep(t *testing.T) {
	Convey("Given a cross-sectional change-breadth instrument", t, func() {
		entity := NewTicker(t.Context())

		Convey("the first tick declares the gate fact with no cohort yet", func() {
			measurement := entity.Step(tick("BTC/USD", 100.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["valid_member_count"].Raw, ShouldEqual, 0.0)
			So(measurement.Maturity, ShouldEqual, 0.0)
		})

		Convey("a quoted market with no trade writes no cohort facts", func() {
			measurement := entity.Step(tick("BTC/USD", 0.0, timestamp(1)))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["valid_member_count"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["last"].Raw, ShouldEqual, 0.0)
		})

		Convey("a measurement without a price fails the gate", func() {
			measurement := tick("BTC/USD", -1.0, timestamp(1))
			measurement = entity.Step(measurement)

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("cohort breadth facts appear once two symbols have changes", func() {
			entity.Step(tick("BTC/USD", 100.0, timestamp(1)))
			entity.Step(tick("ETH/USD", 200.0, timestamp(1)))

			up := entity.Step(tick("BTC/USD", 110.0, timestamp(2)))

			So(up.Metrics["valid_member_count"].Raw, ShouldEqual, 1.0)
			So(up.Metrics["positive_count"].Raw, ShouldEqual, 1.0)
			So(up.Metadata[data.MetadataSupport], ShouldEqual, 1.0)

			down := entity.Step(tick("ETH/USD", 150.0, timestamp(2)))

			So(down.Metrics["valid_member_count"].Raw, ShouldEqual, 2.0)
			So(down.Metrics["positive_count"].Raw, ShouldEqual, 1.0)
			So(down.Metrics["negative_count"].Raw, ShouldEqual, 1.0)
			So(down.Metrics["signed_fraction"].Raw, ShouldEqual, 0.0)
			So(down.Metrics["directional_participation"].Raw, ShouldBeZeroValue)

			So(down.Metadata[data.MetadataSupport], ShouldEqual, 2.0)

			So(down.Provenance["extreme_key"], ShouldNotBeEmpty)

			// The aggregate's causal estimator matures over its own history of
			// cuts, so the cross-section is driven until the view reports.
			prices := map[string]float64{"BTC/USD": 110.0, "ETH/USD": 150.0}

			for second := 3; second < 11; second++ {
				prices["BTC/USD"] += 5.0
				entity.Step(tick("BTC/USD", prices["BTC/USD"], timestamp(int64(second))))

				prices["ETH/USD"] -= 5.0
				settled := entity.Step(tick("ETH/USD", prices["ETH/USD"], timestamp(int64(second))))

				if second > 8 {
					So(settled.Metrics["signed_fraction_zscore"].Raw, ShouldNotBeZeroValue)
				}
			}
		})

		Convey("Register declares the full fact schema without values", func() {
			measurement := entity.Register()

			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "last")
			So(measurement.Metrics, ShouldContainKey, "signed_fraction_zscore")
			So(measurement.Metrics, ShouldContainKey, "valid_member_count")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}
