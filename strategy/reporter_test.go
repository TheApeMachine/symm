package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestReporter(t *testing.T) {
	Convey("Given a strategy Reporter and snapshot", t, func() {
		reporter := NewReporter()

		snapshot := ReportSnapshot{
			Source:       "training",
			Symbol:       "BTC/USD",
			SeqIdx:       42,
			At:           time.Unix(1700000000, 0),
			Stage:        StageHistoricalValidation,
			Blocker:      "evaluating",
			Action:       1,
			Confidence:   0.85,
			Contrast:     0.75,
			Price:        60000.0,
			ExcursionMag: 150.0,
			Direction:    "up",
			Clears:       true,
			Event:        "completed",
			Trading:      true,
			Resolved:     10,
			WinRate:      0.6,
			Edge:         0.02,
		}

		Convey("it extracts valid canonical metadata entries", func() {
			metadata := reporter.Metadata(snapshot)
			So(len(metadata), ShouldBeGreaterThan, 0)

			metaMap := make(map[string]string)
			for _, entry := range metadata {
				metaMap[entry.Key] = entry.Value
			}

			So(metaMap["excursion_direction"], ShouldEqual, "up")
			So(metaMap["stage"], ShouldEqual, "HISTORICAL VALIDATION")
			So(metaMap["stage_blocker"], ShouldEqual, "evaluating")
			So(metaMap["excursion_clears"], ShouldEqual, "true")
			So(metaMap["excursion_event"], ShouldEqual, "completed")
		})

		Convey("it constructs all canonical telemetry metrics", func() {
			metrics := reporter.Metrics(snapshot)
			So(len(metrics), ShouldBeGreaterThan, 0)

			metricMap := make(map[string]float64)
			for _, metric := range metrics {
				metricMap[metric.Label] = metric.Raw
			}

			So(metricMap["steps"], ShouldEqual, 1)
			So(metricMap["decisions"], ShouldEqual, 1)
			So(metricMap["resolved"], ShouldEqual, 10)
			So(metricMap["win_rate"], ShouldEqual, 0.6)
			So(metricMap["edge"], ShouldEqual, 0.02)
			So(metricMap["confidence"], ShouldEqual, 0.85)
			So(metricMap["contrast"], ShouldEqual, 0.75)
			So(metricMap["stage_code"], ShouldEqual, float64(StageHistoricalValidation))
			So(metricMap["trading"], ShouldEqual, 1.0)
			So(metricMap["action"], ShouldEqual, 1.0)
			So(metricMap["excursion_type"], ShouldEqual, 1.0)
			So(metricMap["price"], ShouldEqual, 60000.0)
			So(metricMap["excursion_mag"], ShouldEqual, 150.0)
		})

		Convey("it populates and finalizes an output measurement", func() {
			metadata := reporter.Metadata(snapshot)
			out := data.NewMeasurement(1, "BTC/USD", "training", 42, 42, metadata...)
			out.At = snapshot.At
			out.From = snapshot.At

			finalized := reporter.Populate(out, snapshot)
			So(finalized, ShouldNotBeNil)
			So(finalized.Label, ShouldEqual, "BTC/USD")
			So(finalized.SeqIdx, ShouldEqual, 42)

			val := data.Pull(finalized.Read("steps"))
			So(val.Err, ShouldBeNil)
			So(val.Metric.Raw, ShouldEqual, 1)
		})
	})
}
