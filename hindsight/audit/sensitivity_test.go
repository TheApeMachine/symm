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
			mCVD := data.NewMeasurement(1, "BTC/USD", "cvd", tick, tick)
			mCVD.At = now.Add(time.Duration(tick) * time.Millisecond)
			mCVD.From = mCVD.At
			mCVD = mCVD.Write(data.NewMetric("signed_net_fraction", 0.9, data.UnitDimensionless, data.TimescaleTick))

			mDepth := data.NewMeasurement(1, "BTC/USD", "depthflow", tick, tick)
			mDepth.At = mCVD.At
			mDepth.From = mCVD.From
			mDepth = mDepth.Write(data.NewMetric("ask_retreat_velocity", 0.6, data.UnitDimensionless, data.TimescaleTick))

			mHawkes := data.NewMeasurement(1, "BTC/USD", "hawkes", tick, tick)
			mHawkes.At = mCVD.At
			mHawkes.From = mCVD.From
			mHawkes = mHawkes.Write(data.NewMetric("branching_ratio:buy", 0.4, data.UnitDimensionless, data.TimescaleTick))

			tickMeasurements[tick] = []*data.Measurement{mCVD, mDepth, mHawkes}
		}

		Convey("When auditing grid sensitivity and family dependence", func() {
			report := AnalyzeGridSensitivity(grid, ticks, tickMeasurements)

			So(len(report.FamiliesTested), ShouldEqual, 3)
			So(report.DuplicationResistant, ShouldBeTrue)
			So(report.Passed, ShouldBeTrue)
		})
	})
}
