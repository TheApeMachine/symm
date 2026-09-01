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

/*
TestTickerStep_ZeroPrice pins the zero-price case. A contract that has not
traded reports a price of zero, which is a real market state and not bad data.
Both log and log ratio are undefined there, and an ungated log-space block
failed the whole frame -- discarding every arithmetic price metric alongside
it, for every observation of that symbol.
*/
func TestTickerStep_ZeroPrice(t *testing.T) {
	Convey("Given a contract whose last price is zero", t, func() {
		entity := NewTicker()
		at := time.Unix(1_700_000_000, 0)

		Convey("The first observation still publishes its arithmetic metrics", func() {
			measurement := entity.Step(futuresTicker("PF_UNTRADED", 0, 100, 100.5, 1000, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["reference_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["open_interest"].Raw, ShouldEqual, 1000.0)

			// The arithmetic basis is defined at zero; its log is not.
			_, hasLogBasis := measurement.Metrics["log_basis"]
			So(hasLogBasis, ShouldBeFalse)

			_, hasClosure := measurement.Metrics["basis_closure_error"]
			So(hasClosure, ShouldBeFalse)
		})

		Convey("A second zero-priced observation still reports without error", func() {
			So(entity.Step(futuresTicker("PF_UNTRADED", 0, 100, 100.5, 1000, at)).Err, ShouldBeNil)

			measurement := entity.Step(futuresTicker(
				"PF_UNTRADED", 0, 101, 100.5, 1000, at.Add(time.Second),
			))

			So(measurement.Err, ShouldBeNil)

			_, hasDerivativeReturn := measurement.Metrics["derivative_log_return"]
			So(hasDerivativeReturn, ShouldBeFalse)

			// The reference leg is positive throughout, so it still measures.
			So(measurement.Metrics["reference_log_return"].Raw, ShouldNotEqual, 0.0)

			// One absent leg leaves the gap between them undefined.
			_, hasGap := measurement.Metrics["return_gap"]
			So(hasGap, ShouldBeFalse)
		})

		Convey("A price recovering from zero reports without error", func() {
			So(entity.Step(futuresTicker("PF_UNTRADED", 0, 100, 100.5, 1000, at)).Err, ShouldBeNil)

			measurement := entity.Step(futuresTicker(
				"PF_UNTRADED", 102, 101, 100.5, 1000, at.Add(time.Second),
			))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["derivative_price"].Raw, ShouldEqual, 102.0)

			// The log basis is defined again once both prices are positive.
			So(measurement.Metrics["log_basis"].Raw, ShouldNotEqual, 0.0)

			// The return's previous endpoint was zero, so the ratio stays absent.
			_, hasDerivativeReturn := measurement.Metrics["derivative_log_return"]
			So(hasDerivativeReturn, ShouldBeFalse)
		})
	})

	Convey("Given positive prices throughout", t, func() {
		entity := NewTicker()
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(futuresTicker("PF_XBTUSD", 100, 99, 99.5, 1000, at)).Err, ShouldBeNil)

		Convey("The full log-space geometry is still published", func() {
			measurement := entity.Step(futuresTicker(
				"PF_XBTUSD", 102, 100, 100.5, 1100, at.Add(time.Second),
			))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["log_basis"].Raw, ShouldAlmostEqual, math.Log(102.0/100.0), 1e-12)
			So(measurement.Metrics["derivative_log_return"].Raw, ShouldAlmostEqual, math.Log(102.0/100.0), 1e-12)
			So(measurement.Metrics["reference_log_return"].Raw, ShouldAlmostEqual, math.Log(100.0/99.0), 1e-12)

			// The three-price geometry closes: d-s == (d-i) + (i-s).
			So(measurement.Metrics["basis_closure_error"].Raw, ShouldAlmostEqual, 0.0, 1e-12)
		})
	})
}

func TestTickerStep_RegressingTimestamp(t *testing.T) {
	Convey("Given a snapshot whose timestamp regresses relative to the prior one", t, func() {
		entity := NewTicker()
		at := time.Unix(1_700_000_000, 0)

		Convey("the observer's causal clock must not regress across the out-of-order event", func() {
			So(entity.Step(futuresTicker("PF_XBTUSD", 101, 100, 100.5, 1000, at)).Err, ShouldBeNil)
			So(entity.Step(futuresTicker("PF_XBTUSD", 102, 101, 101.5, 1100, at.Add(time.Second))).Err, ShouldBeNil)

			// A snapshot carrying a REAL timestamp older than the last seen is
			// a late event, not a broken one. Its instantaneous price geometry
			// is true whenever the snapshot was taken, so it publishes; but it
			// is not a valid newest observation, so nothing derived from the
			// event clock does.
			measurement := entity.Step(futuresTicker("PF_XBTUSD", 103, 102, 102.5, 1200, at.Add(500*time.Millisecond)))
			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["derivative_price"].Raw, ShouldEqual, 103.0)
			So(measurement.Metrics["basis"].Raw, ShouldAlmostEqual, (103.0-102.0)/102.0, 1e-12)

			// Not republished from the previous frame under this event's id.
			_, hasChange := measurement.Metrics["open_interest_change"]
			So(hasChange, ShouldBeFalse)
		})

		Convey("a fabricated timestamp is folded forward, not read as late", func() {
			So(entity.Step(futuresTicker("PF_SYNUSD", 101, 100, 100.5, 1000, at.Add(time.Hour))).Err, ShouldBeNil)

			// No server timestamp: the wall-clock substitute reads as older,
			// but it holds no truth, so it is pinned to the timeline head and
			// the snapshot counts as the newest observation.
			point := futuresTicker("PF_SYNUSD", 102, 101, 101.5, 1100, at)
			point.SyntheticTimestamp = true

			measurement := entity.Step(point)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["open_interest_change"].Raw, ShouldEqual, 100.0)
		})

		Convey("identical timestamps are accepted and hold the timeline at the same instant", func() {
			So(entity.Step(futuresTicker("PF_RAREUSD", 101, 100, 100.5, 1000, at)).Err, ShouldBeNil)

			measurement := entity.Step(futuresTicker("PF_RAREUSD", 102, 101, 101.5, 1100, at))
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["open_interest_change"].Raw, ShouldEqual, 100.0)
		})
	})
}
