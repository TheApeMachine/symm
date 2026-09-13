package correlation

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
tick builds the measurement the workload's data management would hand the
signal: the register's declared schema with the feed's last price written.
Zero is an unobserved market; a negative price is an invalid one.
*/
var schema = new(Ticker).Register().Metrics

func tick(symbol string, price float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("correlation", maps.Clone(schema))
	m.Label, m.At, m.From = symbol, at, at
	m.Metrics["last_price"] = m.Metrics["last_price"].Write(price)

	return m
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

func TestTickerStep(t *testing.T) {
	Convey("Given a correlation ticker-path instrument", t, func() {
		entity := NewTicker(t.Context())

		Convey("the first tick yields one measurement with no warmup", func() {
			measurement := entity.Step(tick("BTC/USD", 100.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_correlation"].Raw, ShouldEqual, 0.0)

			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a quoted market with no recent trade does not enter the price path", func() {
			untraded := tick("CORN/USD", 0, timestamp(1))

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

		Convey("a measurement without a price fails the gate", func() {
			measurement := entity.Step(tick("BTC/USD", -1, timestamp(1)))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("cohort and pair-history facts appear once two symbols share paths", func() {
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104})
			measurements := drive(entity, "ETH/USD", []float64{200, 202, 203, 205, 206})

			last := measurements[len(measurements)-1]

			So(last.Metrics, ShouldContainKey, "signed_correlation")
			So(last.Metrics, ShouldContainKey, "absolute_correlation")

			signed := last.Metrics["signed_correlation"].Raw
			So(signed, ShouldBeGreaterThan, 0.0)
			So(signed, ShouldBeLessThan, 1.0)

			So(last.Metrics["cohort_peer_count"].Raw, ShouldEqual, 1.0)
			So(last.Metrics["overlap_pair_count"].Raw, ShouldEqual, 4.0)
			So(last.Metrics["supported_return_count:measured"].Raw, ShouldEqual, 4.0)
			So(last.Metrics["supported_return_count:reference"].Raw, ShouldEqual, 4.0)
			So(last.Metrics["shared_time"].Raw, ShouldAlmostEqual, 4.0, 1e-3)
			So(last.Metrics["overlap_density"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics, ShouldContainKey, "covariance")
			So(last.Metrics, ShouldContainKey, "return_energy:reference")
			So(last.Metrics, ShouldContainKey, "return_energy:measured")
			So(last.Metrics["return_energy_rate:reference"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics["return_energy_rate:measured"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics["peer_return_energy_rate"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics, ShouldContainKey, "correlation_p_value")
			So(last.Metrics, ShouldContainKey, "correlation_standard_error_fisher")
			So(last.Metrics, ShouldContainKey, "cohort_correlation_dispersion")
			So(last.Metrics, ShouldContainKey, "cohort_effective_peer_count")
			So(last.Metrics, ShouldContainKey, "relative_return_energy")

			So(last.Metrics, ShouldContainKey, "correlation_baseline")
			So(last.Metrics, ShouldContainKey, "correlation_divergence")
			So(last.Metrics, ShouldContainKey, "correlation_zscore")
			So(last.Metrics, ShouldContainKey, "correlation_velocity")
			So(last.Metrics, ShouldContainKey, "relative_return_energy_baseline")
		})

		Convey("a settled correlation estimator yields a defined SNR", func() {
			// The signed correlation has to actually move from cut to cut before
			// its Fisher-space estimator has a noise model to report, so the two
			// paths drift in and out of step rather than tracking each other.
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
			So(measurement.Metrics, ShouldContainKey, "last_price")
			So(measurement.Metrics, ShouldContainKey, "signed_correlation")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}

/*
BenchmarkTickerCrossSectionStep isolates the intrinsic cost of one correlation
Step on a focal symbol whose peers all hold full (64-sample) committed paths. It
exercises the CrossSection peer fan-out (one Hayashi pair evaluation per peer)
plus the per-tick cohort reduce/finalize. Sustained single-digit-millisecond
cost here means a ~1s avg on the live diagnostics is contention, not intrinsic
compute.
*/
func BenchmarkTickerCrossSectionStep(b *testing.B) {
	entity := NewTicker(context.Background())

	// Prime every symbol's path to steady-state capacity (64 samples) so the
	// cross-section cost reflects a fully-warmed universe, not cold-start.
	for s := 0; s < benchmarkSymbols; s++ {
		symbol := benchmarkSymbol(s)
		for i := 0; i < benchmarkWarmup; i++ {
			entity.Step(tick(symbol, 100.0+float64(i), timestamp(int64(i)+1)))
		}
	}

	// The register measurement flows through every tick in production, so the
	// benchmark reuses one instead of reallocating per iteration.
	focal := benchmarkSymbol(0)
	measurement := tick(focal, 0, timestamp(benchmarkWarmup))
	i := 0
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		measurement.Metrics["last_price"] = measurement.Metrics["last_price"].Write(100.0 + float64(i))
		measurement.At = timestamp(int64(benchmarkWarmup + i + 1))
		entity.Step(measurement)
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
