package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCategorySurpriseTracker(t *testing.T) {
	Convey("Given a four-category surprise tracker", t, func() {
		Convey("It should score the first selection against the uniform prior", func() {
			tracker := NewCategorySurpriseTracker([]CategoryType{
				CategoryVolumeStarvation,
				CategoryStochasticBalance,
				CategoryHiddenAbsorption,
				CategoryAggressiveDrive,
			}, DefaultCategorySurpriseAlpha)

			score, err := tracker.Score(CategoryAggressiveDrive)
			So(err, ShouldBeNil)
			So(score, ShouldBeGreaterThan, 0)
		})

		Convey("It should decay SNR as a category becomes habitual", func() {
			tracker := NewCategorySurpriseTracker([]CategoryType{
				CategoryVolumeStarvation,
				CategoryStochasticBalance,
				CategoryHiddenAbsorption,
				CategoryAggressiveDrive,
			}, DefaultCategorySurpriseAlpha)

			firstRare, err := tracker.Score(CategoryAggressiveDrive)
			So(err, ShouldBeNil)

			for range 30 {
				_, err := tracker.Score(CategoryAggressiveDrive)
				So(err, ShouldBeNil)
			}

			habitual, err := tracker.Score(CategoryAggressiveDrive)
			So(err, ShouldBeNil)
			So(habitual, ShouldBeLessThan, firstRare)
		})
	})
}

func TestCategorySurpriseField(t *testing.T) {
	Convey("Given a per-symbol surprise field", t, func() {
		field := NewCategorySurpriseField([]CategoryType{
			CategoryLaminar,
			CategoryInertial,
			CategoryViscous,
			CategoryTurbulent,
		}, DefaultCategorySurpriseAlpha)

		for index := range 20 {
			category := CategoryLaminar

			if index%3 == 1 {
				category = CategoryTurbulent
			}

			if index%3 == 2 {
				category = CategoryInertial
			}

			_, err := field.Score("BTC/EUR", category)
			So(err, ShouldBeNil)
		}

		Convey("It should normalize each symbol independently", func() {
			quiet, err := field.Score("BTC/EUR", CategoryLaminar)
			So(err, ShouldBeNil)

			rareBTC, err := field.Score("BTC/EUR", CategoryTurbulent)
			So(err, ShouldBeNil)

			firstDOGE, err := field.Score("DOGE/EUR", CategoryTurbulent)
			So(err, ShouldBeNil)

			So(firstDOGE, ShouldBeGreaterThan, quiet)
			So(rareBTC, ShouldNotAlmostEqual, firstDOGE, 1e-9)
		})
	})
}

func TestAssignCategorySurpriseSNR(t *testing.T) {
	Convey("Given a measurement and surprise field", t, func() {
		field := NewCategorySurpriseField([]CategoryType{
			CategoryVolumeStarvation,
			CategoryStochasticBalance,
			CategoryHiddenAbsorption,
			CategoryAggressiveDrive,
		}, DefaultCategorySurpriseAlpha)

		measurement := Measurement{Symbol: "BTC/EUR"}

		for range defaultSNRMinObs {
			So(AssignCategorySurpriseSNR(&measurement, field, CategoryStochasticBalance), ShouldBeNil)
		}

		Convey("It should spike on an unexpected category, not on repeated selections", func() {
			repeated := Measurement{Symbol: "BTC/EUR"}
			So(AssignCategorySurpriseSNR(&repeated, field, CategoryStochasticBalance), ShouldBeNil)

			rare := Measurement{Symbol: "BTC/EUR"}
			So(AssignCategorySurpriseSNR(&rare, field, CategoryAggressiveDrive), ShouldBeNil)

			So(rare.SNR, ShouldBeGreaterThan, repeated.SNR)
		})
	})
}

const defaultSNRMinObs = 12
