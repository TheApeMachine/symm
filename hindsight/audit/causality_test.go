package audit

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
lookaheadReplay standardizes each value against the mean of the whole input,
future included: the known-bad pipeline the future check must catch.
*/
func lookaheadReplay(measurements []*data.Measurement, _ int64, _ *store.Grid) map[string]float64 {
	sum, count := 0.0, 0.0

	for _, m := range measurements {
		for entry := range m.Read() {
			sum += entry.Metric.Raw
			count++
		}
	}

	out := make(map[string]float64)

	for _, m := range measurements {
		for entry := range m.Read() {
			out[observationKey(m, entry.Key)] = entry.Metric.Raw - sum/count
		}
	}

	return out
}

/*
pooledReplay keeps one running sum across every symbol: the known-bad
pipeline the cross-symbol check must catch.
*/
func pooledReplay(measurements []*data.Measurement, _ int64, _ *store.Grid) map[string]float64 {
	sum := 0.0
	out := make(map[string]float64)

	for _, m := range measurements {
		for entry := range m.Read() {
			sum += entry.Metric.Raw
			out[observationKey(m, entry.Key)] = sum
		}
	}

	return out
}

/*
carriedState keeps state across calls regardless of epoch: the known-bad
pipeline the epoch check must catch.
*/
var carriedState float64

func carriedReplay(measurements []*data.Measurement, _ int64, _ *store.Grid) map[string]float64 {
	out := make(map[string]float64)

	for _, m := range measurements {
		for entry := range m.Read() {
			carriedState += entry.Metric.Raw
			out[observationKey(m, entry.Key)] = carriedState
		}
	}

	return out
}

func TestCausalityAudit(t *testing.T) {
	Convey("Given multi-symbol temporal measurements", t, func() {
		grid := store.NewGrid()
		var ticks []int64
		tickMeasurements := make(map[int64][]*data.Measurement)
		now := time.Unix(1_800_000_000, 0)

		for tick := int64(1); tick <= 40; tick++ {
			ticks = append(ticks, tick)

			for index, symbol := range []string{"BTC/USD", "ETH/USD"} {
				m := data.NewMeasurement(7, symbol, "cvd", tick*2+int64(index), tick)
				m.At = now.Add(time.Duration(tick) * time.Millisecond)
				m.From = m.At
				tickMeasurements[tick] = append(tickMeasurements[tick], m.Write(
					data.NewMetric("signed_net_fraction", float64((tick*7+int64(index)*3)%11)/11-0.5, data.UnitDimensionless, data.TimescaleTick),
				))
			}
		}

		var past, future []*data.Measurement

		for _, tick := range ticks {
			if tick <= 20 {
				past = append(past, tickMeasurements[tick]...)
			} else {
				future = append(future, tickMeasurements[tick]...)
			}
		}

		Convey("the production replay is causal, symbol-isolated and epoch-isolated", func() {
			report := AnalyzeCausality(grid, ticks, tickMeasurements)

			So(report.ComparedObservations, ShouldBeGreaterThan, 0)
			So(report.LeakageDetected, ShouldBeFalse)
			So(report.CrossSymbolLeakage, ShouldBeFalse)
			So(report.EpochIsolationPassed, ShouldBeTrue)
			So(report.Status, ShouldEqual, VerdictValid)
		})

		Convey("a pipeline that looks ahead is caught", func() {
			report := causalityProbe(lookaheadReplay, grid, past, future, 1)
			So(report.LeakageDetected, ShouldBeTrue)
			So(report.Status, ShouldEqual, VerdictBreach)
		})

		Convey("a pipeline that pools symbols is caught", func() {
			report := causalityProbe(pooledReplay, grid, past, future, 1)
			So(report.CrossSymbolLeakage, ShouldBeTrue)
			So(report.Passed, ShouldBeFalse)
		})

		Convey("a pipeline that carries state across epochs is caught", func() {
			report := causalityProbe(carriedReplay, grid, past, future, 1)
			So(report.EpochIsolationPassed, ShouldBeFalse)
			So(report.Status, ShouldEqual, VerdictBreach)
		})
	})
}
