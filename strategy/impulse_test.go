package strategy

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestImpulse_Checkpoint(t *testing.T) {
	Convey("Given a settled Impulse Map", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		impulse := newImpulse(nil)
		stream := store.NewStream()

		base := map[string]float64{
			store.CellKey("cvd_value"):       1.0,
			store.CellKey("hawkes_value"):    2.0,
			store.CellKey("depthflow_value"): 3.0,
			store.CellKey("liquidity_value"): 4.0,
		}

		for tick := int64(1); tick <= 8; tick++ {
			step := make(map[string]float64, len(base))

			for key, value := range base {
				step[key] = value + float64(tick*tick)*0.1
			}

			impulse.grid.Update(tick, stream.Deform(step))
		}

		impulse.grid.Settle()

		settled, err := impulse.checkpoint(ctx, 1, 1)
		So(err, ShouldBeNil)
		So(settled, ShouldBeTrue)

		lit := map[string]store.Excitation{
			store.CellKey("cvd_value"):       {Deformation: 0.25, Confidence: 1},
			store.CellKey("hawkes_value"):    {Deformation: 0.5, Confidence: 1},
			store.CellKey("depthflow_value"): {Deformation: 0.75, Confidence: 1},
			store.CellKey("liquidity_value"): {Deformation: 0.125, Confidence: 1},
		}
		before := impulse.grid.LitRegion(lit)
		So(before, ShouldNotBeEmpty)

		Convey("Snapshot and RestoreSnapshot preserve the settled region tokens", func() {
			encoded, err := impulse.grid.Snapshot()
			So(err, ShouldBeNil)

			restored := newImpulse(nil)
			So(restored.grid.IsSettled(), ShouldBeFalse)
			So(restored.grid.RestoreSnapshot(encoded), ShouldBeNil)
			So(restored.grid.IsSettled(), ShouldBeTrue)
			So(restored.grid.LitRegion(lit), ShouldResemble, before)
		})
	})
}

func TestImpulse_Restored(t *testing.T) {
	Convey("Given an Impulse Map whose catalog has no object storage", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		impulse := newImpulse(tablestest.New(t))

		Convey("restored reports no checkpoint, not an error, every time", func() {
			for range 3 {
				restored, err := impulse.restored(ctx)
				So(err, ShouldBeNil)
				So(restored, ShouldBeFalse)
			}
		})
	})

	Convey("Given an Impulse Map with no catalog", t, func() {
		restored, err := newImpulse(nil).restored(context.Background())
		So(err, ShouldBeNil)
		So(restored, ShouldBeFalse)
	})
}

func TestChannelsFrom_KeysByMetricLabel(t *testing.T) {
	Convey("Given the same metric from two symbols and two producers", t, func() {
		at := time.Now().UTC()
		write := func(label, source string, raw float64) *data.Measurement {
			measurement := data.NewMeasurement(1, label, source, 1, 1)
			measurement.At = at
			measurement.From = at
			return measurement.Write(
				data.NewMetric("spread", raw, data.UnitCount, data.TimescaleTick),
			)
		}

		channels := channelsFrom(
			write("BTC/USD", "liquidity", 1),
			write("ETH/USD", "pumpdump", 2),
		)

		Convey("They map onto one cell keyed by the metric label alone", func() {
			So(channels.raw, ShouldHaveLength, 1)
			So(channels.raw, ShouldContainKey, store.CellKey("spread"))
		})
	})
}

func TestChannelsFrom_CollapsesPeerQualifiedFacts(t *testing.T) {
	Convey("Given one correlation fact published against three peer symbols", t, func() {
		at := time.Now().UTC()
		measurement := data.NewMeasurement(1, "BTC/USD", "correlation", 1, 1)
		measurement.At = at
		measurement.From = at
		measurement = measurement.Write(
			data.NewMetric("signed_correlation@ETH/USD", 0.2, data.UnitCorrelation, data.TimescaleRollingWindow),
			data.NewMetric("signed_correlation@SOL/USD", 0.4, data.UnitCorrelation, data.TimescaleRollingWindow),
			data.NewMetric("signed_correlation@XRP/USD", 0.9, data.UnitCorrelation, data.TimescaleRollingWindow),
			data.NewMetric("cohort_peer_count", 3, data.UnitCount, data.TimescaleInstantaneous),
		)

		channels := channelsFrom(measurement)

		Convey("They land in one fact cell holding the mean across peers", func() {
			So(channels.raw, ShouldHaveLength, 2)
			So(channels.raw["signed_correlation"], ShouldAlmostEqual, 0.5, 1e-12)
			So(channels.raw["cohort_peer_count"], ShouldEqual, 3)
		})
	})
}

func TestChannelsFrom_CarriesMeasurementConfidence(t *testing.T) {
	Convey("Given a trusted and an immature Measurement in one pass", t, func() {
		at := time.Now().UTC()
		write := func(source string, measurement *data.Measurement, metrics ...*data.Metric) *data.Measurement {
			measurement.At = at
			measurement.From = at
			return measurement.Write(metrics...)
		}

		trusted := write("liquidity", data.NewMeasurement(1, "BTC/USD", "liquidity", 1, 1).Restore(2, 0.5),
			data.NewMetric("spread", 1, data.UnitCount, data.TimescaleTick),
			data.NewMetric("depth", 2, data.UnitCount, data.TimescaleTick),
		)
		immature := write("cvd", data.NewMeasurement(1, "BTC/USD", "cvd", 1, 1),
			data.NewMetric("cvd_value", 3, data.UnitCount, data.TimescaleTick),
		)

		channels := channelsFrom(trusted, immature)

		Convey("Every metric of a Measurement shares its confidence", func() {
			So(channels.confidence["spread"], ShouldAlmostEqual, trusted.Confidence(), 1e-12)
			So(channels.confidence["depth"], ShouldAlmostEqual, trusted.Confidence(), 1e-12)
			So(channels.confidence["cvd_value"], ShouldEqual, 0)
		})

		Convey("excite pairs each deformation with its cell's confidence", func() {
			pass := channels.excite(map[string]float64{"spread": 0.5, "cvd_value": 0.25})

			So(pass, ShouldHaveLength, 2)
			So(pass["spread"], ShouldResemble, store.Excitation{Deformation: 0.5, Confidence: trusted.Confidence()})
			So(pass["cvd_value"], ShouldResemble, store.Excitation{Deformation: 0.25})
		})
	})
}
