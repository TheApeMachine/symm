package causal

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

const nanosPerStep = int64(1_000_000_000)

func buildHY(prices []float64) *hyReturns {
	series := newHYReturns(256)

	for index, price := range prices {
		series.Observe(int64(index+1)*nanosPerStep, price)
	}

	return series
}

func TestHayashiYoshidaPerfectComovement(t *testing.T) {
	Convey("Given two assets whose synchronous returns move identically", t, func() {
		left := buildHY([]float64{100, 101, 103, 102, 104, 103, 105})
		right := make([]float64, 0)
		base := 50.0

		// Right asset: same log returns as left, different price level.
		ratios := []float64{1, 1.01, 103.0 / 101.0, 102.0 / 103.0, 104.0 / 102.0, 103.0 / 104.0, 105.0 / 103.0}
		price := base

		for _, ratio := range ratios {
			price *= ratio
			right = append(right, price)
		}

		rightSeries := buildHY(right)

		Convey("The Hayashi-Yoshida correlation should be ~1", func() {
			correlation, ok := hayashiYoshidaCorrelation(left, rightSeries)

			So(ok, ShouldBeTrue)
			So(correlation, ShouldAlmostEqual, 1, 0.0001)
		})
	})
}

func TestHayashiYoshidaAntiComovement(t *testing.T) {
	Convey("Given two assets whose synchronous returns are mirror images", t, func() {
		left := buildHY([]float64{100, 102, 101, 104, 103})
		right := buildHY([]float64{100, 98, 99, 96, 97})

		Convey("The correlation should be strongly negative", func() {
			correlation, ok := hayashiYoshidaCorrelation(left, right)

			So(ok, ShouldBeTrue)
			So(correlation, ShouldBeLessThan, -0.9)
		})
	})
}

func TestHayashiYoshidaDisjointIntervalsHaveNoCovariance(t *testing.T) {
	Convey("Given two series whose return intervals never overlap in time", t, func() {
		left := newHYReturns(16)
		left.Observe(1*nanosPerStep, 100)
		left.Observe(2*nanosPerStep, 101)

		right := newHYReturns(16)
		right.Observe(5*nanosPerStep, 50)
		right.Observe(6*nanosPerStep, 51)

		Convey("The asynchronous covariance should be zero", func() {
			So(hayashiYoshidaCovariance(left.intervals, right.intervals), ShouldEqual, 0)
		})
	})
}

func TestHayashiYoshidaFlatSeriesReportsNoCorrelation(t *testing.T) {
	Convey("Given a series with no price movement", t, func() {
		left := buildHY([]float64{100, 100, 100, 100})
		right := buildHY([]float64{50, 51, 52, 53})

		Convey("It should refuse to manufacture a correlation", func() {
			_, ok := hayashiYoshidaCorrelation(left, right)
			So(ok, ShouldBeFalse)
		})
	})
}

func TestHayashiYoshidaToleratesAsynchronousTicks(t *testing.T) {
	Convey("Given two series sampled at offset, non-aligned timestamps", t, func() {
		left := newHYReturns(64)
		right := newHYReturns(64)

		// Left ticks on even nanoseconds, right on odd — overlapping but never identical.
		leftPrices := []float64{100, 101, 102, 103, 104, 105}
		rightPrices := []float64{200, 202, 204, 206, 208, 210}

		for index := range leftPrices {
			left.Observe(int64(2*index+1)*nanosPerStep, leftPrices[index])
			right.Observe(int64(2*index+2)*nanosPerStep, rightPrices[index])
		}

		Convey("Overlapping returns should still yield a positive correlation", func() {
			correlation, ok := hayashiYoshidaCorrelation(left, right)

			So(ok, ShouldBeTrue)
			So(correlation, ShouldBeGreaterThan, 0)
			So(math.Abs(correlation), ShouldBeLessThanOrEqualTo, 1)
		})
	})
}
