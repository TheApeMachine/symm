package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func pumpdumpTouch(symbol string, bid float64, ask float64, at time.Time) *data.Measurement[float64] {
	metrics := make(map[string]data.Metric[float64])

	if bid > 0 {
		metrics["best_bid"] = data.NewMetric[float64]("best_bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(bid)
	}

	if ask > 0 {
		metrics["best_ask"] = data.NewMetric[float64]("best_ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(ask)
	}

	m := data.NewMeasurement[float64]("websocket", metrics)
	m.Label, m.At, m.From = symbol, at, at

	return m
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a message with an executable touch", t, func() {
		entity := NewLevel3(t.Context())
		at := time.Unix(1_700_000_000, 0)

		Convey("Step derives the touch from the message's own orders", func() {
			measurement := entity.Step(pumpdumpTouch("BTC/USD", 99, 101, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)

			So(measurement.Maturity, ShouldEqual, 1.0)
		})
	})

	Convey("Given a symbol whose book has never shown both sides", t, func() {
		entity := NewLevel3(t.Context())
		at := time.Unix(1_700_000_000, 0)

		Convey("Step yields no measurement rather than an error", func() {
			So(entity.Step(pumpdumpTouch("MISSING", 0, 0, at)), ShouldBeNil)
		})

		Convey("A one-sided message alone still yields no measurement", func() {
			So(entity.Step(pumpdumpTouch("ONESIDED", 99, 0, at)), ShouldBeNil)
		})
	})

	Convey("Given a symbol that has seen both sides across separate messages", t, func() {
		entity := NewLevel3(t.Context())
		at := time.Unix(1_700_000_000, 0)

		// First observation carries bid only
		So(entity.Step(pumpdumpTouch("BTC/USD", 99, 0, at)), ShouldBeNil)

		Convey("A later one-sided update borrows the retained opposite side", func() {
			measurement := entity.Step(pumpdumpTouch("BTC/USD", 0, 101, at.Add(time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
		})
	})
}

func TestLevel3Register(t *testing.T) {
	Convey("Given a Level3 entity", t, func() {
		entity := NewLevel3(t.Context())
		schema := entity.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "pumpdump:level3")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"best_bid",
			"best_ask",
			"midpoint",
			"spread",
			"relative_spread",
		}

		for _, name := range expected {
			metric, ok := schema.Metrics[name]
			So(ok, ShouldBeTrue)
			So(metric.Label, ShouldEqual, name)
			So(metric.Raw, ShouldEqual, 0.0)
		}
	})
}
