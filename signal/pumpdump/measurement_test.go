package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var benchmarkMeasurement *nmtypes.Measurement

func TestSignalMeasurement(t *testing.T) {
	Convey("Given a classified PumpDump frame", t, func() {
		at := time.Unix(1_786_099_210, 250).UTC()
		observedFrom := at.Add(-5 * time.Second)
		output := pumpDumpOutputForTest(at, observedFrom, true)
		measurement := (&Signal{}).measurement(at, output)

		Convey("It preserves the estimator provenance and interval", func() {
			So(measurement.Source, ShouldEqual, string(types.SourcePumpDump))
			So(measurement.At, ShouldResemble, at)
			So(measurement.ObservedFrom, ShouldResemble, observedFrom)
			So(measurement.Horizon, ShouldEqual, 5*time.Second)
			So(measurement.Maturity, ShouldEqual, 0.75)
			So(measurement.Metadata, ShouldResemble, map[string]float64{
				"capacity":        128,
				"volume":          3,
				"last":            101,
				"bid":             100,
				"ask":             102,
				"unix_sec":        float64(at.Unix()),
				"unix_nsec":       float64(at.Nanosecond()),
				"bar_rate":        60,
				"rate_baseline":   30,
				"spread_baseline": 4,
			})
		})

		Convey("It publishes the complete side-aware current evidence set", func() {
			So(len(measurement.Metrics), ShouldEqual, 13)
			assertMetric(t, measurement, string(types.MetricRVOL), 2, 2.0/3.0)
			assertMetric(t, measurement, string(types.MetricSpread), 2, 2.0/101.0)
			assertMetric(t, measurement, string(types.MetricCompression), 0.5, 0.5)
			assertMetric(t, measurement,
				types.MetricKey(types.MetricPrecursor, types.SideBuy), 1.5, 0.6)
			assertMetric(t, measurement,
				types.MetricKey(types.MetricPrecursor, types.SideSell), 0.25, 0.2)
			assertMetric(t, measurement,
				types.MetricKey(types.MetricExhaustion, types.SideBuy), 0.8, 0.8)
			assertMetric(t, measurement,
				types.MetricKey(types.MetricExhaustion, types.SideSell), 0.2, 0.2)
			assertMetric(t, measurement,
				string(types.MetricHypothesisSeparation), 0.75, 0.75)
			So(measurement.Metrics[types.MetricKey(types.MetricBestPrice, types.SideBuy)].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics[types.MetricKey(types.MetricBestPrice, types.SideSell)].Raw, ShouldEqual, 102.0)
			So(measurement.Metrics[string(types.MetricMidpoint)].Raw,
				ShouldEqual, 101.0)
			So(measurement.Metrics[string(types.MetricTradePrice)].Raw,
				ShouldEqual, 101.0)
			So(measurement.Metrics[string(types.MetricTradeQuantity)].Raw,
				ShouldEqual, 3.0)
		})
	})

	Convey("Given a provisional PumpDump frame", t, func() {
		at := time.Unix(1_786_099_210, 250).UTC()
		output := pumpDumpOutputForTest(at, time.Time{}, false)
		measurement := (&Signal{}).measurement(at, output)

		Convey("It remains explicit without publishing unready evidence", func() {
			So(measurement.ObservedFrom, ShouldResemble, at)
			So(measurement.Horizon, ShouldEqual, time.Duration(0))
			So(measurement.Metrics[string(types.MetricRVOL)].Normalized,
				ShouldBeNil)
			So(measurement.Metrics[types.MetricKey(types.MetricPrecursor, types.SideBuy)].Normalized, ShouldBeNil)
			So(measurement.Metrics[string(types.MetricHypothesisSeparation)].Normalized, ShouldBeNil)
			So(measurement.Metrics[string(types.MetricSpread)].Normalized,
				ShouldNotBeNil)
			So(measurement.Metrics[string(types.MetricCompression)].Normalized,
				ShouldBeNil)
		})
	})
}

func pumpDumpOutputForTest(
	at time.Time,
	observedFrom time.Time,
	ready bool,
) nomagique.Frame {
	output := nomagique.Frame{}
	output.Put(algo.SymbolCapacity, 128)
	output.Put(algo.SymbolVolume, 3)
	output.Put(algo.SymbolLast, 101)
	output.Put(algo.SymbolBid, 100)
	output.Put(algo.SymbolAsk, 102)
	output.Put(algo.SymbolUnixSec, float64(at.Unix()))
	output.Put(algo.SymbolUnixNsec, float64(at.Nanosecond()))
	observedFromSeconds := 0.0
	observedFromNanoseconds := 0.0

	if !observedFrom.IsZero() {
		observedFromSeconds = float64(observedFrom.Unix())
		observedFromNanoseconds = float64(observedFrom.Nanosecond())
	}

	output.Put(algo.SymbolIgnitionObservedFromSec, observedFromSeconds)
	output.Put(algo.SymbolIgnitionObservedFromNsec, observedFromNanoseconds)
	output.Put(algo.SymbolIgnitionBarRate, 60)
	output.Put(algo.SymbolIgnitionRateBaseline, 30)
	output.Put(algo.SymbolRVOL, 2)
	output.Put(algo.SymbolRVOLNormalized, 2.0/3.0)
	output.Put(algo.SymbolMidpoint, 101)
	output.Put(algo.SymbolSpread, 2)
	output.Put(algo.SymbolSpreadNormalized, 2.0/101.0)
	output.Put(algo.SymbolSpreadBaseline, 4)
	output.Put(algo.SymbolCompression, 0.5)
	output.Put(algo.SymbolMaturity, 0.75)
	output.Put(algo.SymbolIgnitionHypothesisSeparation, 0.75)
	output.Put(algo.SymbolAlphaPrecursor, 1.5)
	output.Put(algo.SymbolAlphaPrecursorNormalized, 0.6)
	output.Put(algo.SymbolBetaPrecursor, 0.25)
	output.Put(algo.SymbolBetaPrecursorNormalized, 0.2)
	output.Put(algo.SymbolAlphaExhaustion, 0.8)
	output.Put(algo.SymbolBetaExhaustion, 0.2)
	output.Put(algo.SymbolReady, boolNumberForTest(ready))

	return output
}

func assertMetric(
	t *testing.T,
	measurement *nmtypes.Measurement,
	name string,
	raw float64,
	normalized float64,
) {
	t.Helper()
	metric, found := measurement.Metrics[name]
	So(found, ShouldBeTrue)
	So(metric.Raw, ShouldAlmostEqual, raw)
	So(metric.Normalized, ShouldNotBeNil)
	So(*metric.Normalized, ShouldAlmostEqual, normalized)
}

func boolNumberForTest(value bool) float64 {
	if value {
		return 1
	}

	return 0
}

func BenchmarkSignalMeasurement(b *testing.B) {
	at := time.Unix(1_786_099_210, 250).UTC()
	output := pumpDumpOutputForTest(at, at.Add(-5*time.Second), true)
	signal := &Signal{}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		benchmarkMeasurement = signal.measurement(at, output)
	}
}
