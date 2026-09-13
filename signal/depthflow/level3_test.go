package depthflow

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

var baseTime = time.Unix(1_700_000_000, 0)

func row(
	symbol string,
	obsBid, obsAsk float64,
	addBid, addAsk float64,
	modBid, modAsk float64,
	delBid, delAsk float64,
	mutBid, mutAsk float64,
	at time.Time,
) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
		"observed_notional:bid":         data.NewMetric[float64]("observed_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(obsBid),
		"observed_notional:ask":         data.NewMetric[float64]("observed_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(obsAsk),
		"add_notional:bid":              data.NewMetric[float64]("add_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(addBid),
		"add_notional:ask":              data.NewMetric[float64]("add_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(addAsk),
		"modify_remaining_notional:bid": data.NewMetric[float64]("modify_remaining_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(modBid),
		"modify_remaining_notional:ask": data.NewMetric[float64]("modify_remaining_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(modAsk),
		"delete_count:bid":              data.NewMetric[float64]("delete_count:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(delBid),
		"delete_count:ask":              data.NewMetric[float64]("delete_count:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(delAsk),
		"mutation_count:bid":            data.NewMetric[float64]("mutation_count:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(mutBid),
		"mutation_count:ask":            data.NewMetric[float64]("mutation_count:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(mutAsk),
	})
	m.Label, m.At, m.From = symbol, at, at

	return m
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a streaming depth-flow signal", t, func() {
		level3 := NewLevel3(t.Context())

		Convey("a snapshot is reduced to facts carried by that one message", func() {
			measurement := level3.Step(row(
				"BTC/USD",
				296, 202,
				296, 202,
				0, 0,
				0, 0,
				2, 1,
				baseTime,
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["observed_notional:bid"].Raw, ShouldAlmostEqual, 296)
			So(measurement.Metrics["observed_notional:ask"].Raw, ShouldAlmostEqual, 202)
			So(measurement.Metrics["observed_notional_imbalance"].Raw, ShouldAlmostEqual, 94.0/498.0, 1e-9)
			So(measurement.Metrics["mutation_count:bid"].Raw, ShouldEqual, 2)
			So(measurement.Metrics["mutation_count:ask"].Raw, ShouldEqual, 1)
		})

		Convey("the next message does not inherit untouched orders", func() {
			level3.Step(row(
				"BTC/USD",
				198, 202,
				198, 202,
				0, 0,
				0, 0,
				1, 1,
				baseTime,
			))

			measurement := level3.Step(row(
				"BTC/USD",
				100, 0,
				100, 0,
				0, 0,
				0, 0,
				1, 0,
				baseTime.Add(time.Second),
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["observed_notional:bid"].Raw, ShouldEqual, 100)
			So(measurement.Metrics["observed_notional:ask"].Raw, ShouldEqual, 0)
			So(measurement.Metrics["observed_notional"].Raw, ShouldEqual, 100)
			So(measurement.Metrics["add_notional:bid"].Raw, ShouldEqual, 100)
			So(measurement.Metrics["mutation_activity_imbalance"].Raw, ShouldEqual, 1)
			So(measurement.Metrics["observed_notional_rate"].Raw, ShouldEqual, 100)
		})

		Convey("modify and delete retain only facts the wire actually supplies", func() {
			measurement := level3.Step(row(
				"ETH/USD",
				150, 0,
				0, 0,
				150, 0,
				1, 1,
				2, 1,
				baseTime,
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["modify_remaining_notional:bid"].Raw, ShouldEqual, 150)
			So(measurement.Metrics["delete_count:bid"].Raw, ShouldEqual, 1)
			So(measurement.Metrics["delete_count:ask"].Raw, ShouldEqual, 1)
		})
	})
}

func TestLevel3Register(t *testing.T) {
	Convey("Given a Level3 entity", t, func() {
		level3 := NewLevel3(t.Context())
		schema := level3.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "depthflow:level3")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"observed_notional:bid",
			"observed_notional:ask",
			"observed_notional",
			"observed_notional_diff",
			"add_notional:bid",
			"add_notional:ask",
			"modify_remaining_notional:bid",
			"modify_remaining_notional:ask",
			"delete_count:bid",
			"delete_count:ask",
			"mutation_count:bid",
			"mutation_count:ask",
			"mutation_count",
			"mutation_count_diff",
			"mutation_activity_imbalance",
			"observed_notional_imbalance",
			"observed_notional_rate",
			"observed_notional_imbalance_baseline",
			"observed_notional_imbalance_divergence",
			"observed_notional_imbalance_zscore",
			"observed_notional_rate_baseline",
			"observed_notional_rate_divergence",
			"observed_notional_rate_zscore",
		}

		for _, name := range expected {
			metric, ok := schema.Metrics[name]
			So(ok, ShouldBeTrue)
			So(metric.Label, ShouldEqual, name)
			So(metric.Raw, ShouldEqual, 0.0)
		}
	})
}
