package derivatives

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func futuresTicker(
	symbol string,
	last float64,
	index float64,
	openInterest float64,
	at time.Time,
) kraken.FuturesTickerData {
	return kraken.FuturesTickerData{
		Symbol:       symbol,
		Last:         decimal.NewFromFloat64(last),
		IndexPrice:   decimal.NewFromFloat64(index),
		OpenInterest: openInterest,
		Timestamp:    at,
	}
}

func TestTickerStep(t *testing.T) {
	Convey("Given a valid derivative ticker snapshot", t, func() {
		entity := NewTicker()
		at := time.Unix(1_700_000_000, 0)

		Convey("the first data point yields point metrics with no warmup", func() {
			measurement := entity.Step(futuresTicker("PF_XBTUSD", 101, 100, 1000, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)

			So(measurement.Metrics["derivative_price"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["reference_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["open_interest"].Raw, ShouldEqual, 1000.0)
			So(measurement.Metrics["basis"].Raw, ShouldAlmostEqual, 0.01, 1e-12)
			So(measurement.Metrics["log_basis"].Raw, ShouldAlmostEqual, math.Log(101.0/100.0), 1e-12)

			// No previous observation exists yet, so first-difference and
			// baseline metrics are omitted.
			_, hasChange := measurement.Metrics["open_interest_change"]
			So(hasChange, ShouldBeFalse)
			_, hasGrowth := measurement.Metrics["open_interest_growth_rate"]
			So(hasGrowth, ShouldBeFalse)

			// A stateless direct measurement is whole; no noise model means
			// SNR is undefined (0), derived rather than caller-set.
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a multi-leg sequence derives the open-interest dynamics", func() {
			entity.Step(futuresTicker("PF_XBTUSD", 101, 100, 1000, at))
			measurement := entity.Step(futuresTicker("PF_XBTUSD", 102, 101, 1100, at.Add(10*time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["open_interest_change"].Raw, ShouldAlmostEqual, 100.0, 1e-12)
			So(measurement.Metrics["open_interest_log_change"].Raw, ShouldAlmostEqual, math.Log(1100.0/1000.0), 1e-12)
			So(measurement.Metrics["open_interest_growth_rate"].Raw, ShouldAlmostEqual, math.Log(1100.0/1000.0)/10.0, 1e-12)
			So(measurement.Metrics["basis"].Raw, ShouldAlmostEqual, (102.0-101.0)/101.0, 1e-12)

			// The first baseline is seeded by the first growth observation, so
			// the departure is zero and the z-score is zero.
			So(measurement.Metrics["open_interest_growth_baseline"].Raw, ShouldAlmostEqual, math.Log(1100.0/1000.0)/10.0, 1e-12)
			So(measurement.Metrics["open_interest_growth_zscore"].Raw, ShouldEqual, 0.0)

			// One retained estimator sample is still immature.
			So(measurement.Maturity, ShouldEqual, 0.0)
		})
	})

	Convey("Given a derivative ticker with a non-positive reference price", t, func() {
		entity := NewTicker()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(futuresTicker("PF_XBTUSD", 101, 0, 1000, time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
