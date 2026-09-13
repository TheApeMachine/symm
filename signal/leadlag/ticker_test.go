package leadlag

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	markettest "github.com/theapemachine/symm/tests/market"
)

/*
tick builds the measurement the workload's data management would hand the
signal: the register's declared schema with the feed's last trade price
written. Zero is an unobserved market; a negative price is an invalid one.
*/
var schema = new(Ticker).Register().Metrics

func tick(symbol string, price float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("leadlag", cloneSchema())
	m.Label, m.At, m.From = symbol, at, at
	m.Metrics["last"] = m.Metrics["last"].Write(price)

	return m
}

func cloneSchema() map[string]data.Metric[float64] {
	metrics := make(map[string]data.Metric[float64], len(schema))

	for key, metric := range schema {
		metrics[key] = metric
	}

	return metrics
}

func timestamp(second int64) time.Time {
	return time.Unix(1_700_000_000+second, 0)
}

func drive(entity *Ticker, symbol string, prices []float64) []*data.Measurement[float64] {
	measurements := make([]*data.Measurement[float64], 0, len(prices))

	for index, price := range prices {
		measurements = append(measurements, entity.Step(tick(
			symbol, price, timestamp(int64(index)+1),
		)))
	}

	return measurements
}

func tapeTicks() []*data.Measurement[float64] {
	measurements := make([]*data.Measurement[float64], 0, 32)

	for _, row := range markettest.LeadLagTape() {
		measurements = append(measurements, tick(row.Symbol, row.Last.Float64(), row.Timestamp))
	}

	return measurements
}

func TestTickerStep(t *testing.T) {
	Convey("Given the captured CRV/DOT tape that stalled the spot workload", t, func() {
		entity := NewTicker(context.Background())
		var measurement *data.Measurement[float64]

		Convey("Every asynchronous observation completes, including the boundary lag", func() {
			for _, arrival := range tapeTicks() {
				measurement = entity.Step(arrival)
				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldBeNil)
			}

			So(measurement.Metrics["best_lag_index"].Raw, ShouldEqual, -5)
			So(measurement.Metrics["lag_peak_curvature"].Raw, ShouldEqual, 0.0)
		})
	})

	Convey("Given a lead-lag ticker-path instrument", t, func() {
		entity := NewTicker(context.Background())

		Convey("the first tick yields one measurement with no warmup", func() {
			measurement := entity.Step(tick("BTC/USD", 100.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["best_lag_correlation"].Raw, ShouldEqual, 0.0)

			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a quoted market with no recent trade does not enter the price path", func() {
			untraded := tick("CORN/USD", 0, timestamp(1))
			untraded.Metrics["bid"] = untraded.Metrics["bid"].Write(0.02015)
			untraded.Metrics["ask"] = untraded.Metrics["ask"].Write(0.04414)

			measurement := entity.Step(untraded)

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last_price"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["observation_count"].Raw, ShouldEqual, 0.0)
			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.Provenance["last_trade_price_state"], ShouldEqual, "unobserved")

			observed := entity.Step(tick("CORN/USD", 0.03, timestamp(2)))
			unobservedAgain := entity.Step(tick("CORN/USD", 0, timestamp(3)))
			observedAgain := entity.Step(tick("CORN/USD", 0.033, timestamp(4)))

			So(observed.Err, ShouldBeNil)
			So(observed.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
			So(observed.Metrics["last_price"].Raw, ShouldEqual, 0.03)
			So(unobservedAgain.Err, ShouldBeNil)
			So(unobservedAgain.Metrics["observation_count"].Raw, ShouldEqual, 0.0)
			So(observedAgain.Err, ShouldBeNil)
			So(observedAgain.Metrics["observation_count"].Raw, ShouldEqual, 2.0)
		})

		Convey("a negative last price remains invalid", func() {
			measurement := entity.Step(tick("BTC/USD", -1, timestamp(1)))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("best-lag and pair-history facts appear once CrossLag is ready", func() {
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104, 105})
			measurements := drive(entity, "ETH/USD", []float64{200, 202, 204, 206, 208, 210})

			last := measurements[len(measurements)-1]

			So(last.Metrics["contemporaneous_correlation"].Raw, ShouldNotEqual, 0.0)
			So(last.Metrics["best_lag_correlation"].Raw, ShouldNotEqual, 0.0)
			So(last.Metrics["best_lag_seconds"].Raw, ShouldNotEqual, 0.0)
			So(last.Metrics["absolute_correlation_gain"].Raw, ShouldNotEqual, 0.0)
			So(last.Metrics["lag_search_resolution_seconds"].Raw, ShouldAlmostEqual, 1.0, 1e-9)

			So(last.Metrics["reference_return_count"].Raw, ShouldEqual, 6.0)
			So(last.Metrics["measured_return_count"].Raw, ShouldEqual, 6.0)
			So(last.Metrics["effective_sample_count"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics["search_count"].Raw, ShouldBeGreaterThan, 0.0)

			So(last.Metrics["lag_baseline_seconds"].Raw, ShouldNotEqual, 0.0)
			So(last.Metrics["correlation_gain_baseline"].Raw, ShouldNotEqual, 0.0)
			So(last.Metrics["best_lag_correlation_baseline"].Raw, ShouldNotEqual, 0.0)

			// The correlation history is the estimator fact the measurement's
			// support derives from: a defined pair history means support.
			So(last.Metadata[data.MetadataSupport], ShouldBeGreaterThan, 0.0)
		})

		Convey("the pair history yields defined divergences once a prior exists", func() {
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104, 105})
			measurements := drive(entity, "ETH/USD", []float64{200, 202, 201, 205, 203, 208, 204, 211, 206, 214, 208, 217})

			last := measurements[len(measurements)-1]

			// The drifted tape moves the best lag from cut to cut, so the
			// lag and gain histories carry real divergence and velocity.
			So(last.Metrics["lag_divergence_seconds"].Raw, ShouldNotEqual, 0.0)
			So(last.Metrics["correlation_gain_zscore"].Raw, ShouldNotEqual, 0.0)
			So(last.Metadata[data.MetadataDivergence], ShouldNotEqual, 0.0)
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
			entity.Step(tick("BTC/USD", 100.0, timestamp(2)))

			measurement := entity.Step(tick("BTC/USD", 101.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metadata[data.MetadataSupport], ShouldEqual, 0)
			So(measurement.Provenance["event_time_state"], ShouldEqual, "regressed")
		})

		Convey("Register declares the full metric schema without values", func() {
			measurement := entity.Register()

			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "last")
			So(measurement.Metrics, ShouldContainKey, "best_lag_correlation")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}

func BenchmarkTickerStep(b *testing.B) {
	arrivals := tapeTicks()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		entity := NewTicker(context.Background())

		for _, arrival := range arrivals {
			measurement := entity.Step(arrival)

			if measurement.Err != nil {
				b.Fatal(measurement.Err)
			}
		}
	}
}

func benchmarkSymbol(s int) string {
	return "S" + string(rune('0'+s/10)) + string(rune('0'+s%10)) + "/USD"
}

const (
	benchmarkSymbols = 32
	benchmarkWarmup  = 64
)

/*
BenchmarkTickerCrossLagStep isolates the intrinsic cost of one leadlag Step on a
focal symbol whose peers all hold full (64-sample) committed paths. It exercises
the pair fan-out (one lag-surface scan per peer) plus the per-tick pair
history/finalize. Sustained single-digit-millisecond cost here means a ~1s avg
on the live diagnostics is contention, not intrinsic compute.
*/
func BenchmarkTickerCrossLagStep(b *testing.B) {
	entity := NewTicker(context.Background())

	// Prime every symbol's path to steady-state capacity (64 samples) so the
	// cross-section cost reflects a fully-warmed universe, not cold-start.
	for s := 0; s < benchmarkSymbols; s++ {
		symbol := benchmarkSymbol(s)

		for i := 0; i < benchmarkWarmup; i++ {
			entity.Step(tick(symbol, 100.0+float64(i), timestamp(int64(i)+1)))
		}
	}

	focal := benchmarkSymbol(0)
	i := 0
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		entity.Step(tick(focal, 100.0+float64(i), timestamp(int64(benchmarkWarmup+i)+1)))
		i++
	}
}
