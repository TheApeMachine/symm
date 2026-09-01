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
	mark float64,
	openInterest float64,
	at time.Time,
) kraken.FuturesTickerData {
	return kraken.FuturesTickerData{
		Symbol:       symbol,
		Last:         decimal.NewFromFloat64(last),
		IndexPrice:   decimal.NewFromFloat64(index),
		MarkPrice:    decimal.NewFromFloat64(mark),
		OpenInterest: openInterest,
		Timestamp:    at,
	}
}

func TestTickerStep(t *testing.T) {
	Convey("Given a valid derivative ticker snapshot", t, func() {
		entity := NewTicker()
		at := time.Unix(1_700_000_000, 0)

		Convey("the first data point yields point and geometry metrics with no warmup", func() {
			measurement := entity.Step(futuresTicker("PF_XBTUSD", 101, 100, 100.5, 1000, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)

			So(measurement.Metrics["derivative_price"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["reference_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["open_interest"].Raw, ShouldEqual, 1000.0)
			So(measurement.Metrics["basis"].Raw, ShouldAlmostEqual, 0.01, 1e-12)
			So(measurement.Metrics["log_basis"].Raw, ShouldAlmostEqual, math.Log(101.0/100.0), 1e-12)

			// Three-price log basis geometry is a closed identity.
			So(measurement.Metrics["derivative_index_log_basis"].Raw, ShouldAlmostEqual, math.Log(101.0/100.0), 1e-12)
			So(measurement.Metrics["index_spot_log_basis"].Raw, ShouldAlmostEqual, math.Log(100.0/100.5), 1e-12)
			So(measurement.Metrics["derivative_spot_log_basis"].Raw, ShouldAlmostEqual, math.Log(101.0/100.5), 1e-12)
			So(measurement.Metrics["basis_closure_error"].Raw, ShouldAlmostEqual, 0.0, 1e-12)

			// The first baseline is the observation itself; the z-score is not
			// yet estimable without a prior baseline.
			So(measurement.Metrics["basis_baseline"].Raw, ShouldAlmostEqual, 0.01, 1e-12)
			_, hasBasisZ := measurement.Metrics["basis_zscore"]
			So(hasBasisZ, ShouldBeFalse)

			// No previous observation exists yet, so first-difference and
			// baseline metrics are omitted.
			_, hasChange := measurement.Metrics["open_interest_change"]
			So(hasChange, ShouldBeFalse)
			_, hasGrowth := measurement.Metrics["open_interest_growth_rate"]
			So(hasGrowth, ShouldBeFalse)
			_, hasBasisChange := measurement.Metrics["basis_change"]
			So(hasBasisChange, ShouldBeFalse)
			_, hasDerivativeReturn := measurement.Metrics["derivative_log_return"]
			So(hasDerivativeReturn, ShouldBeFalse)
			_, hasReturnGap := measurement.Metrics["return_gap"]
			So(hasReturnGap, ShouldBeFalse)

			// No primary estimator has run yet, so the point measurement is whole.
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a multi-leg sequence derives the differences, returns, and baselines", func() {
			entity.Step(futuresTicker("PF_XBTUSD", 101, 100, 100.5, 1000, at))
			measurement := entity.Step(futuresTicker("PF_XBTUSD", 102, 101, 101.5, 1100, at.Add(10*time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["open_interest_change"].Raw, ShouldAlmostEqual, 100.0, 1e-12)
			So(measurement.Metrics["open_interest_log_change"].Raw, ShouldAlmostEqual, math.Log(1100.0/1000.0), 1e-12)
			So(measurement.Metrics["open_interest_growth_rate"].Raw, ShouldAlmostEqual, math.Log(1100.0/1000.0)/10.0, 1e-12)
			So(measurement.Metrics["open_interest_growth_baseline"].Raw, ShouldAlmostEqual, math.Log(1100.0/1000.0)/10.0, 1e-12)

			So(measurement.Metrics["basis_change"].Raw, ShouldAlmostEqual, (102.0-101.0)/101.0-0.01, 1e-12)
			So(measurement.Metrics["basis_rate"].Raw, ShouldAlmostEqual, ((102.0-101.0)/101.0-0.01)/10.0, 1e-12)
			So(measurement.Metrics["derivative_log_return"].Raw, ShouldAlmostEqual, math.Log(102.0/101.0), 1e-12)
			So(measurement.Metrics["reference_log_return"].Raw, ShouldAlmostEqual, math.Log(101.0/100.0), 1e-12)
			So(measurement.Metrics["return_gap"].Raw, ShouldAlmostEqual, math.Log(102.0/101.0)-math.Log(101.0/100.0), 1e-12)

			// The basis z-score's first dispersion is the residual itself, so a
			// decline below its seeded baseline scores -1.
			So(measurement.Metrics["basis_zscore"].Raw, ShouldAlmostEqual, -1.0, 1e-9)

			// One retained estimator sample is still immature.
			So(measurement.Maturity, ShouldEqual, 0.0)
		})
	})

	Convey("Given a derivative ticker with a non-positive reference price", t, func() {
		entity := NewTicker()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(futuresTicker("PF_XBTUSD", 101, 0, 100.5, 1000, time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}

/*
TestTickerStep_ZeroOpenInterest pins the zero-open-interest case. A contract
nobody holds reports an open interest of zero, which is a real market state and
not bad data. log(current/previous) is undefined at either endpoint being zero,
and an ungated LogRatio failed the whole frame -- discarding every price metric
alongside it.
*/
func TestTickerStep_ZeroOpenInterest(t *testing.T) {
	Convey("Given a contract whose open interest is zero", t, func() {
		entity := NewTicker()
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(futuresTicker("PF_THIN", 101, 100, 100.5, 0, at)).Err, ShouldBeNil)

		Convey("A second observation still publishes its price metrics", func() {
			measurement := entity.Step(futuresTicker(
				"PF_THIN", 102, 100, 100.5, 0, at.Add(time.Second),
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["derivative_price"].Raw, ShouldEqual, 102.0)

			// The arithmetic change is defined at zero; its log is not.
			So(measurement.Metrics["open_interest_change"].Raw, ShouldEqual, 0.0)

			_, hasLogChange := measurement.Metrics["open_interest_log_change"]
			So(hasLogChange, ShouldBeFalse)
		})
	})

	Convey("Given open interest that rises from zero", t, func() {
		entity := NewTicker()
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(futuresTicker("PF_OPEN", 101, 100, 100.5, 0, at)).Err, ShouldBeNil)

		Convey("The log change stays absent while the previous endpoint is zero", func() {
			measurement := entity.Step(futuresTicker(
				"PF_OPEN", 101, 100, 100.5, 500, at.Add(time.Second),
			))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["open_interest_change"].Raw, ShouldEqual, 500.0)

			_, hasLogChange := measurement.Metrics["open_interest_log_change"]
			So(hasLogChange, ShouldBeFalse)
		})
	})
}
