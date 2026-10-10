package audit

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestSensitivityAudit(t *testing.T) {
	Convey("Given multi-family signal tape", t, func() {
		grid := store.NewGrid()
		ticks := []int64{1, 2, 3, 4, 5}
		tickMeasurements := make(map[int64][]*data.Measurement)

		now := time.Now()
		for _, tick := range ticks {
			mCVD := data.NewMeasurement(9101, "BTC/USD", "cvd", tick, tick)
			mCVD.At = now.Add(time.Duration(tick) * time.Millisecond)
			mCVD.From = mCVD.At
			mCVD = mCVD.Write(data.NewMetric("signed_net_fraction", 0.9, data.UnitDimensionless, data.TimescaleTick))

			mDepth := data.NewMeasurement(9101, "BTC/USD", "depthflow", tick, tick)
			mDepth.At = mCVD.At
			mDepth.From = mCVD.From
			mDepth = mDepth.Write(data.NewMetric("ask_retreat_velocity", 0.6, data.UnitDimensionless, data.TimescaleTick))

			mHawkes := data.NewMeasurement(9101, "BTC/USD", "hawkes", tick, tick)
			mHawkes.At = mCVD.At
			mHawkes.From = mCVD.From
			mHawkes = mHawkes.Write(data.NewMetric("branching_ratio:buy", 0.4, data.UnitDimensionless, data.TimescaleTick))

			tickMeasurements[tick] = []*data.Measurement{mCVD, mDepth, mHawkes}
		}

		Convey("When auditing grid sensitivity and family dependence", func() {
			report := AnalyzeGridSensitivity(grid, ticks, tickMeasurements, DefaultThresholds())

			So(len(report.FamiliesTested), ShouldEqual, 3)
			// Constant metrics never get a defined z, so the grid lights
			// nothing: that is no evidence of balance.
			So(report.Status, ShouldEqual, VerdictInsufficient)
			So(report.Passed, ShouldBeFalse)
		})

		Convey("When one family's removal moves the regions past the limit", func() {
			stats := []FamilySensitivityStat{
				{Family: "cvd", RemovedJSD: 0.9, IsDominant: true},
				{Family: "hawkes", RemovedJSD: 0.01},
			}
			report := sensitivityReport(stats, 1, 0.01, "cvd", DefaultThresholds())
			So(report.Status, ShouldEqual, VerdictNotSupported)
		})

		Convey("When duplicating a family moves the regions past the limit", func() {
			stats := []FamilySensitivityStat{{Family: "cvd"}, {Family: "hawkes"}}
			report := sensitivityReport(stats, 1, 0.6, "cvd", DefaultThresholds())
			So(report.DuplicationResistant, ShouldBeFalse)
			So(report.Status, ShouldEqual, VerdictNotSupported)

			balanced := sensitivityReport(stats, 1, 0.01, "cvd", DefaultThresholds())
			So(balanced.Status, ShouldEqual, VerdictSupported)
		})
	})
}
