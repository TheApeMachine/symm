package audit

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestCausalityAudit(t *testing.T) {
	Convey("Given multi-symbol temporal measurements", t, func() {
		grid := store.NewGrid()
		ticks := []int64{1, 2, 3, 4, 5, 6, 7, 8}
		tickMeasurements := make(map[int64][]*data.Measurement)

		now := time.Now()
		for _, tick := range ticks {
			mBTC := data.NewMeasurement(1, "BTC/USD", "cvd", tick, tick)
			mBTC.At = now.Add(time.Duration(tick) * time.Millisecond)
			mBTC.From = mBTC.At
			mBTC = mBTC.Write(data.NewMetric("signed_net_fraction", 0.6, data.UnitDimensionless, data.TimescaleTick))

			mETH := data.NewMeasurement(1, "ETH/USD", "cvd", tick, tick)
			mETH.At = mBTC.At
			mETH.From = mBTC.From
			mETH = mETH.Write(data.NewMetric("signed_net_fraction", -0.4, data.UnitDimensionless, data.TimescaleTick))

			tickMeasurements[tick] = []*data.Measurement{mBTC, mETH}
		}

		Convey("When auditing temporal causality and cross-symbol isolation", func() {
			report := AnalyzeCausality(grid, ticks, tickMeasurements)

			So(report.LeakageDetected, ShouldBeFalse)
			So(report.ContaminatedCount, ShouldEqual, 0)
			So(report.CrossSymbolLeakage, ShouldBeFalse)
			So(report.EpochIsolationPassed, ShouldBeTrue)
			So(report.Passed, ShouldBeTrue)
		})
	})
}
