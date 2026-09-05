package leadlag

import (
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

func ticker(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      decimal.NewFromFloat64(price),
		Timestamp: at,
	}
}

func timestamp(second int64) time.Time {
	return time.Unix(1_700_000_000+second, 0)
}

func drive(entity *Ticker, symbol string, prices []float64) []*data.Measurement[float64] {
	measurements := make([]*data.Measurement[float64], 0, len(prices))

	for index, price := range prices {
		measurements = append(measurements, entity.Step(ticker(
			symbol, price, timestamp(int64(index)+1),
		)))
	}

	return measurements
}

func TestTickerStep(t *testing.T) {
	Convey("Given a lead-lag ticker-path instrument", t, func() {
		entity := NewTicker()

		Convey("the first tick yields one measurement with no warmup", func() {
			measurement := entity.Step(ticker("BTC/USD", 100.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics, ShouldNotContainKey, "best_lag_correlation")

			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a quoted market with no recent trade does not enter the price path", func() {
			untraded := ticker("CORN/USD", 0, timestamp(1))
			untraded.Bid = decimal.NewFromFloat64(0.02015)
			untraded.Ask = decimal.NewFromFloat64(0.04414)

			measurement := entity.Step(untraded)

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotContainKey, "last_price")
			So(measurement.Metrics, ShouldNotContainKey, "observation_count")
			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.Provenance["last_trade_price_state"], ShouldEqual, "unobserved")

			observed := entity.Step(ticker("CORN/USD", 0.03, timestamp(2)))
			untraded.Timestamp = timestamp(3)
			unobservedAgain := entity.Step(untraded)
			observedAgain := entity.Step(ticker("CORN/USD", 0.033, timestamp(4)))

			So(observed.Err, ShouldBeNil)
			So(observed.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
			So(observed.Metrics["last_price"].Raw, ShouldEqual, 0.03)
			So(unobservedAgain.Err, ShouldBeNil)
			So(unobservedAgain.Metrics, ShouldNotContainKey, "observation_count")
			So(observedAgain.Err, ShouldBeNil)
			So(observedAgain.Metrics["observation_count"].Raw, ShouldEqual, 2.0)
		})

		Convey("a negative last price remains invalid", func() {
			measurement := entity.Step(ticker("BTC/USD", -1, timestamp(1)))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("best-lag and pair-history facts appear once CrossLag is ready", func() {
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104, 105})
			measurements := drive(entity, "ETH/USD", []float64{200, 202, 204, 206, 208, 210})

			last := measurements[len(measurements)-1]

			So(last.Metrics, ShouldContainKey, "contemporaneous_correlation")
			So(last.Metrics, ShouldContainKey, "best_lag_correlation")
			So(last.Metrics, ShouldContainKey, "best_lag_index")
			So(last.Metrics, ShouldContainKey, "best_lag_seconds")
			So(last.Metrics, ShouldContainKey, "absolute_correlation_gain")
			So(last.Metrics["lag_search_resolution_seconds"].Raw, ShouldAlmostEqual, 1.0, 1e-9)
			So(last.Metrics, ShouldContainKey, "lag_search_span")

			So(last.Metrics["reference_return_count"].Raw, ShouldEqual, 5.0)
			So(last.Metrics["measured_return_count"].Raw, ShouldEqual, 5.0)
			So(last.Metrics, ShouldContainKey, "overlap_pair_count")
			So(last.Metrics, ShouldContainKey, "effective_sample_count")
			So(last.Metrics["search_count"].Raw, ShouldBeGreaterThan, 0.0)

			So(last.Metrics, ShouldContainKey, "lag_peak_prominence")
			So(last.Metrics, ShouldContainKey, "lag_peak_curvature")
			So(last.Metrics, ShouldContainKey, "correlation_p_value")
			So(last.Metrics, ShouldContainKey, "search_adjusted_p_value")

			So(last.Metrics, ShouldContainKey, "lag_baseline_seconds")
			So(last.Metrics, ShouldContainKey, "lag_divergence_seconds")
			So(last.Metrics, ShouldContainKey, "lag_noise_scale_seconds")
			So(last.Metrics, ShouldContainKey, "lag_zscore")
			So(last.Metrics, ShouldContainKey, "lag_velocity")
			So(last.Metrics, ShouldContainKey, "correlation_gain_baseline")
			So(last.Metrics, ShouldContainKey, "correlation_gain_zscore")
			So(last.Metrics, ShouldContainKey, "correlation_gain_velocity")
			So(last.Metrics, ShouldContainKey, "best_lag_correlation_baseline")
			So(last.Metrics, ShouldContainKey, "best_lag_correlation_zscore")
		})

		Convey("a settled best-lag estimator yields a defined SNR", func() {
			// The best-lag correlation has to actually move from cut to cut before
			// its estimator has a noise model to report, so the two paths drift in
			// and out of step rather than tracking each other exactly.
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111})
			measurements := drive(entity, "ETH/USD", []float64{200, 202, 201, 205, 203, 208, 204, 211, 206, 214, 208, 217})

			last := measurements[len(measurements)-1]

			So(last, ShouldNotBeNil)
			So(last.Err, ShouldBeNil)
			So(last.SNRDefined, ShouldBeTrue)
			So(last.SNR, ShouldBeGreaterThanOrEqualTo, 0.0)
		})

		Convey("time regression surfaces as zero support without error", func() {
			entity.Step(ticker("BTC/USD", 100.0, timestamp(2)))

			measurement := entity.Step(ticker("BTC/USD", 101.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metadata[data.MetadataSupport], ShouldEqual, 0)
			So(measurement.Provenance["event_time_state"], ShouldEqual, "regressed")
		})
	})
}

/*
BenchmarkTickerCrossLagStep isolates the intrinsic cost of one leadlag Step on a
focal symbol whose peers all hold full (64-sample) committed paths. It exercises
the CrossSection peer fan-out (one CrossLag lag-surface scan per peer) plus the
per-tick cohort reduce/finalize. Sustained single-digit-millisecond cost here
means a ~1s avg on the live diagnostics is contention, not intrinsic compute.
*/
func BenchmarkTickerCrossLagStep(b *testing.B) {
	entity := NewTicker()

	// Prime every symbol's path to steady-state capacity (64 samples) so the
	// cross-section cost reflects a fully-warmed universe, not cold-start.
	for s := 0; s < benchmarkSymbols; s++ {
		symbol := benchmarkSymbol(s)
		for i := 0; i < benchmarkWarmup; i++ {
			entity.Step(ticker(symbol, 100.0+float64(i), timestamp(int64(i)+1)))
		}
	}

	focal := benchmarkSymbol(0)
	i := 0
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		entity.Step(ticker(focal, 100.0+float64(i), timestamp(int64(benchmarkWarmup+i)+1)))
		i++
	}
}

func benchmarkSymbol(s int) string {
	return fmt.Sprintf("S%02d/USD", s)
}

const (
	benchmarkSymbols = 32
	benchmarkWarmup  = 64
)
