package correlation

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func TestCorrelationSignalMetrics(t *testing.T) {
	Convey("Given a READY correlation signal", t, func() {
		now := time.Now()
		measurement := data.NewMeasurement(
			now.UnixNano(),
			"BTC/USD",
			"spot:trade",
			system.SeqIdx.Add(1),
			system.Tick.Add(1),
		)
		measurement.At = now
		measurement.From = now
		btcPrice := decimal.NewFromFloat64(50000)
		measurement.Write(
			data.NewExactMetric("price", btcPrice, data.UnitCurrency, data.TimescaleInstantaneous),
		)

		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)
		result := signal.Step(measurement)

		Convey("Then the result should have the expected metrics", func() {
			So(result, ShouldNotBeNil)

			metrics := make([]float64, 0)

			for metricEntry := range result.Read() {
				metrics = append(metrics, metricEntry.Metric.Raw)
			}

			// Single-asset initial tick produces last_price and observation_count;
			// the trade's own price stays on the trade frame.
			So(len(metrics), ShouldEqual, 2)
			So(result.Maturity(), ShouldBeLessThan, 0.1)
		})

		Convey("When a second peer symbol arrives, it correlates against the first", func() {
			secondMeasurement := data.NewMeasurement(
				now.Add(time.Second).UnixNano(),
				"ETH/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			secondMeasurement.At = now.Add(time.Second)
			secondMeasurement.From = now.Add(time.Second)
			ethPrice := decimal.NewFromFloat64(3000)
			secondMeasurement.Write(
				data.NewExactMetric("price", ethPrice, data.UnitCurrency, data.TimescaleInstantaneous),
			)

			secondResult := signal.Step(secondMeasurement)
			So(secondResult, ShouldNotBeNil)
			So(secondResult.Error(), ShouldBeNil)

			secondMetrics := make([]float64, 0)

			for metricEntry := range secondResult.Read() {
				secondMetrics = append(secondMetrics, metricEntry.Metric.Raw)
			}

			So(len(secondMetrics), ShouldEqual, 2)
		})

		Convey("When repeated observations arrive with zero return energy, no NaN metrics surface", func() {
			repeatBtcMeasurement := data.NewMeasurement(
				now.Add(2*time.Second).UnixNano(),
				"BTC/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			repeatBtcMeasurement.At = now.Add(2 * time.Second)
			repeatBtcMeasurement.From = now.Add(2 * time.Second)
			repeatBtcPrice := decimal.NewFromFloat64(50000)
			repeatBtcMeasurement.Write(
				data.NewExactMetric("price", repeatBtcPrice, data.UnitCurrency, data.TimescaleInstantaneous),
			)

			repeatBtcResult := signal.Step(repeatBtcMeasurement)
			So(repeatBtcResult, ShouldNotBeNil)
			So(repeatBtcResult.Error(), ShouldBeNil)

			obsCount := data.Pull(repeatBtcResult.Read("observation_count")).Metric.Raw
			So(obsCount, ShouldEqual, 2)
			lastPrice := data.Pull(repeatBtcResult.Read("last_price")).Metric.Raw
			So(lastPrice, ShouldEqual, 50000)
		})

		Convey("When prices move and return energy is valid, correlation is defined", func() {
			eth1 := data.NewMeasurement(
				now.Add(500*time.Millisecond).UnixNano(),
				"ETH/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			eth1.At = now.Add(500 * time.Millisecond)
			eth1.From = eth1.At
			eth1.Write(
				data.NewExactMetric("price", decimal.NewFromFloat64(3000), data.UnitCurrency, data.TimescaleInstantaneous),
			)
			So(signal.Step(eth1), ShouldNotBeNil)

			btc2 := data.NewMeasurement(
				now.Add(1500*time.Millisecond).UnixNano(),
				"BTC/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			btc2.At = now.Add(1500 * time.Millisecond)
			btc2.From = btc2.At
			btc2.Write(
				data.NewExactMetric("price", decimal.NewFromFloat64(50500), data.UnitCurrency, data.TimescaleInstantaneous),
			)
			So(signal.Step(btc2), ShouldNotBeNil)

			eth2 := data.NewMeasurement(
				now.Add(2000*time.Millisecond).UnixNano(),
				"ETH/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			eth2.At = now.Add(2000 * time.Millisecond)
			eth2.From = eth2.At
			eth2.Write(
				data.NewExactMetric("price", decimal.NewFromFloat64(3100), data.UnitCurrency, data.TimescaleInstantaneous),
			)
			So(signal.Step(eth2), ShouldNotBeNil)

			btc3 := data.NewMeasurement(
				now.Add(3000*time.Millisecond).UnixNano(),
				"BTC/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			btc3.At = now.Add(3000 * time.Millisecond)
			btc3.From = btc3.At
			btc3.Write(
				data.NewExactMetric("price", decimal.NewFromFloat64(51000), data.UnitCurrency, data.TimescaleInstantaneous),
			)

			movingBtcResult := signal.Step(btc3)
			So(movingBtcResult, ShouldNotBeNil)
			So(movingBtcResult.Error(), ShouldBeNil)

			movingMetrics := make([]float64, 0)
			movingKeys := make(map[string]bool)

			for metricEntry := range movingBtcResult.Read() {
				movingMetrics = append(movingMetrics, metricEntry.Metric.Raw)
				movingKeys[metricEntry.Key] = true
			}

			// effective_sample_count, focal_return_energy_rate, and
			// relative_cohort_return_energy were duplicates and are not emitted.
			So(len(movingMetrics), ShouldBeGreaterThanOrEqualTo, 21)

			for _, duplicate := range []string{
				"effective_sample_count", "focal_return_energy_rate", "relative_cohort_return_energy",
			} {
				So(movingKeys[duplicate], ShouldBeFalse)
			}

			signedCorr := data.Pull(movingBtcResult.Read("signed_correlation")).Metric.Raw
			So(signedCorr, ShouldBeBetweenOrEqual, -1.0, 1.0)
			absCorr := data.Pull(movingBtcResult.Read("absolute_correlation")).Metric.Raw
			So(absCorr, ShouldBeBetweenOrEqual, 0.0, 1.0)
			// Too little history for a dispersion: no z-score, so no path
			// point either, rather than zeros standing in for them.
			for _, undefined := range []string{
				"correlation_zscore", "relative_return_energy_zscore",
				"historical_path_distance", "historical_path_percentile",
			} {
				So(movingKeys[undefined], ShouldBeFalse)
			}
		})
	})
}

func TestCorrelationSignalWithoutPrice(t *testing.T) {
	Convey("A frame without a price yields no measurement instead of a panic", t, func() {
		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)

		frame := data.NewMeasurement(1, "BTC/USD", "spot:trade", system.SeqIdx.Add(1), system.Tick.Add(1))
		frame.At = time.Now()
		frame.From = frame.At

		So(func() { So(signal.Step(frame.Write()), ShouldBeNil) }, ShouldNotPanic)
	})
}
