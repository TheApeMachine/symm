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
			So(metricMap["fragments_up"], ShouldEqual, 0)
			So(metricMap["fragments_up_friction"], ShouldEqual, 0)
			So(metricMap["fragments_down"], ShouldEqual, 0)
			So(metricMap["fragments_chop"], ShouldEqual, 0)
			So(metricMap["fragments_flat"], ShouldEqual, 0)
			So(metricMap["fragments_unsupported"], ShouldEqual, 0)
			So(metricMap["price"], ShouldEqual, 60000.0)
			So(metricMap["excursion_mag"], ShouldEqual, 150.0)
		})

		Convey("it maps all five excursion classes onto excursion_type and fragment counters", func() {
			cases := []struct {
				class string
				code  float64
				key   string
			}{
				{"up", 1.0, "fragments_up"},
				{"up_friction", 5.0, "fragments_up_friction"},
				{"down", 2.0, "fragments_down"},
				{"chop", 3.0, "fragments_chop"},
				{"flat", 4.0, "fragments_flat"},
			}

			for _, tc := range cases {
				reporter.RecordFragment(tc.class)
				snap := snapshot
				snap.Direction = tc.class
				metrics := reporter.Metrics(snap)
				metricMap := make(map[string]float64)
				for _, metric := range metrics {
					metricMap[metric.Label] = metric.Raw
				}
				So(metricMap["excursion_type"], ShouldEqual, tc.code)
				So(metricMap[tc.key], ShouldBeGreaterThan, 0)
			}

			reporter.RecordFragment("mystery")
			metrics := reporter.Metrics(snapshot)
			metricMap := make(map[string]float64)
			for _, metric := range metrics {
				metricMap[metric.Label] = metric.Raw
			}
			So(metricMap["fragments_up"], ShouldEqual, 1)
			So(metricMap["fragments_up_friction"], ShouldEqual, 1)
			So(metricMap["fragments_down"], ShouldEqual, 1)
			So(metricMap["fragments_chop"], ShouldEqual, 1)
			So(metricMap["fragments_flat"], ShouldEqual, 1)
			So(metricMap["fragments_unsupported"], ShouldEqual, 1)
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

			metric, err := readMetric(finalized, "steps")
			So(err, ShouldBeNil)
			So(metric, ShouldNotBeNil)
			So(metric.Raw, ShouldEqual, 1)
		})

		Convey("it renders concise high-density summary text and structured report data", func() {
			snapshot.GridCells = 48
			snapshot.GridRegions = 12

			summary := reporter.Summary(snapshot)
			So(summary, ShouldContainSubstring, "[learning] stage=HISTORICAL VALIDATION")
			So(summary, ShouldContainSubstring, `blocker="evaluating"`)
			So(summary, ShouldContainSubstring, "grid=[cells:48 regions:12]")
			So(summary, ShouldContainSubstring, "paper=[trades:10 win_rate:60.0% edge:+2.00%]")

			out := data.NewMeasurement(1, "BTC/USD", "training", 42, 42)
			out.At = snapshot.At
			out.From = snapshot.At
			reporter.Publish(out, snapshot)

			So(reporter.LatestSummary(), ShouldEqual, summary)

			report := reporter.ReportData().(LearningReportData)
			So(report.Stage, ShouldEqual, "HISTORICAL VALIDATION")
			So(report.Blocker, ShouldEqual, "evaluating")
			So(report.Grid.Cells, ShouldEqual, 48)
			So(report.Grid.Regions, ShouldEqual, 12)
			So(report.Paper.Trading, ShouldBeTrue)
			So(report.Paper.Resolved, ShouldEqual, 10)
			So(report.Paper.WinRate, ShouldEqual, 0.6)
			So(report.Paper.Edge, ShouldEqual, 0.02)
		})
	})
}
