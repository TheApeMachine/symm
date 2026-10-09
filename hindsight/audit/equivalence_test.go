package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestEquivalenceAudit(t *testing.T) {
	Convey("Given recorded multi-signal tape", t, func() {
		grid := store.NewGrid()
		ticks := []int64{1, 2, 3, 4, 5}
		tickMeasurements := make(map[int64][]*data.Measurement)

		now := time.Now()
		for _, tick := range ticks {
			m1 := data.NewMeasurement(1, "BTC/USD", "cvd", tick, tick)
			m1.At = now.Add(time.Duration(tick) * time.Millisecond)
			m1.From = m1.At
			m1 = m1.Write(data.NewMetric("signed_net_fraction", 0.8, data.UnitDimensionless, data.TimescaleTick))

			m2 := data.NewMeasurement(1, "BTC/USD", "depthflow", tick, tick)
			m2.At = m1.At
			m2.From = m1.From
			m2 = m2.Write(data.NewMetric("ask_retreat_velocity", 0.5, data.UnitDimensionless, data.TimescaleTick))

			tickMeasurements[tick] = []*data.Measurement{m1, m2}
		}

		Convey("When auditing production-vs-audit equivalence", func() {
			report := AnalyzeEquivalence(context.Background(), grid, ticks, tickMeasurements)

			So(report.TotalTicksReplayed, ShouldEqual, 5)
			So(report.TotalTokensChecked, ShouldEqual, 5)
			So(report.TokenMismatches, ShouldEqual, 0)
			So(report.MetricMismatches, ShouldEqual, 0)
			So(report.Passed, ShouldBeTrue)
			So(len(report.Discrepancies), ShouldEqual, 0)
		})
	})
}
