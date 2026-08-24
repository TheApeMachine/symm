package correlation

import (
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
	Convey("Given a correlation ticker-path instrument", t, func() {
		entity := NewTicker()

		Convey("the first tick yields one measurement with no warmup", func() {
			measurement := entity.Step(ticker("BTC/USD", 100.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics, ShouldNotContainKey, "signed_correlation")

			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.SNR, ShouldEqual, 0.0)
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
			So(last.Metrics["focal_return_energy_rate"].Raw, ShouldBeGreaterThan, 0.0)
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

		Convey("time regression surfaces as measurement.Err", func() {
			entity.Step(ticker("BTC/USD", 100.0, timestamp(2)))

			measurement := entity.Step(ticker("BTC/USD", 101.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
