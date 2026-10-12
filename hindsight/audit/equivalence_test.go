package audit

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestAnalyzeEquivalence(t *testing.T) {
	Convey("Given recorded multi-signal streams chained through the real finalization path", t, func() {
		grid := store.NewGrid()
		warmup := int(core.MinimumRankPrior)
		scored := warmup
		ticks := make([]int64, 0, warmup+scored)
		tickMeasurements := make(map[int64][]*data.Measurement)
		at := time.Unix(1_700_000_000, 0).UTC()

		streams := []struct{ source, label string }{
			{"cvd", "quantity:buy_0"},
			{"depthflow", "removed:ask_0"},
		}

		peers := make([]*data.Measurement, len(streams))

		for step := range warmup + scored {
			tick := int64(step + 1)
			ticks = append(ticks, tick)
			stamp := at.Add(time.Duration(step) * time.Second)

			for index, stream := range streams {
				// Strictly increasing, so every scored step lies beyond all of
				// its priors and carries a nonzero z-score.
				value := math.Sqrt(float64(step + index + 2))

				if peers[index] == nil {
					unit, timescale := data.GetDimension(stream.source, stream.label)
					peer := data.NewMeasurement(1, "BTC/USD", stream.source, tick, tick)
					peer.At, peer.From = stamp, stamp
					peers[index] = peer.Write(data.NewMetric(stream.label, value, unit, timescale))

					continue
				}

				peers[index] = peers[index].Next(stamp, map[string]float64{stream.label: value})
			}

			tickMeasurements[tick] = append([]*data.Measurement(nil), peers...)
		}

		Convey("When auditing production-vs-audit equivalence", func() {
			report := AnalyzeEquivalence(context.Background(), grid, ticks, tickMeasurements)

			So(report.TotalTicksReplayed, ShouldEqual, warmup+scored)
			// A stream's first MinimumRankPrior observations have no defined
			// z-score, so the grid scores exactly the ticks after them.
			So(report.TotalTokensChecked, ShouldEqual, scored)
			So(report.TokenMismatches, ShouldEqual, 0)
			So(report.MetricMismatches, ShouldEqual, 0)
			// One code path compared with itself cannot fail; it is reported
			// as a determinism observation, never as a pass.
			So(report.Deterministic, ShouldBeTrue)
			So(report.Status, ShouldEqual, VerdictNotATest)
			So(report.Passed, ShouldBeFalse)
			So(len(report.Discrepancies), ShouldEqual, 0)
		})
	})
}
