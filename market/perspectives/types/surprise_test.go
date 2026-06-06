package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCategorySurpriseTracker(t *testing.T) {
	Convey("Given a four-category surprise tracker", t, func() {
		Convey("It should score the first selection against the uniform prior", func() {
			tracker, err := NewCategorySurpriseTracker([]CategoryType{
				CategoryVolumeStarvation,
				CategoryStochasticBalance,
				CategoryHiddenAbsorption,
				CategoryAggressiveDrive,
			}, DefaultCategorySurpriseAlpha)

			So(err, ShouldBeNil)

			score, err := tracker.Score(CategoryAggressiveDrive)
			So(err, ShouldBeNil)
			So(score, ShouldBeGreaterThan, 0)
		})

		Convey("It should renormalize when an unseen category is added", func() {
			tracker, err := NewCategorySurpriseTracker([]CategoryType{
				CategoryVolumeStarvation,
				CategoryStochasticBalance,
				CategoryHiddenAbsorption,
				CategoryAggressiveDrive,
			}, DefaultCategorySurpriseAlpha)

			So(err, ShouldBeNil)

			_, err = tracker.Score(CategoryLaminar)
			So(err, ShouldBeNil)

			var total float64
			for _, prob := range tracker.probs {
				total += prob
			}

			So(total, ShouldAlmostEqual, 1.0, 1e-9)
		})

		Convey("It should decay SNR as a category becomes habitual", func() {
			tracker, err := NewCategorySurpriseTracker([]CategoryType{
				CategoryVolumeStarvation,
				CategoryStochasticBalance,
				CategoryHiddenAbsorption,
				CategoryAggressiveDrive,
			}, DefaultCategorySurpriseAlpha)

			So(err, ShouldBeNil)

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

		Convey("It should reject invalid alpha", func() {
			_, err := NewCategorySurpriseTracker([]CategoryType{
				CategoryVolumeStarvation,
			}, 0)

			So(err, ShouldNotBeNil)
		})

		Convey("It should regularize a long-absent category without making it impossible", func() {
			tracker, err := NewCategorySurpriseTracker([]CategoryType{
				CategoryVolumeStarvation,
				CategoryStochasticBalance,
			}, DefaultCategorySurpriseAlpha)

			So(err, ShouldBeNil)
			tracker.probs[CategoryVolumeStarvation] = 0

			score, err := tracker.Score(CategoryVolumeStarvation)
			So(err, ShouldBeNil)
			So(score, ShouldBeGreaterThan, 0)
			So(tracker.probs[CategoryVolumeStarvation], ShouldBeGreaterThan, 0)
		})
	})
}

func TestCategorySurpriseField(t *testing.T) {
	Convey("Given a per-symbol surprise field", t, func() {
		field, err := NewCategorySurpriseField([]CategoryType{
			CategoryLaminar,
			CategoryInertial,
			CategoryViscous,
			CategoryTurbulent,
		}, DefaultCategorySurpriseAlpha)

		So(err, ShouldBeNil)

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
		field, err := NewCategorySurpriseField([]CategoryType{
			CategoryVolumeStarvation,
			CategoryStochasticBalance,
			CategoryHiddenAbsorption,
			CategoryAggressiveDrive,
		}, DefaultCategorySurpriseAlpha)

		So(err, ShouldBeNil)

		measurement := Measurement{Symbol: "BTC/EUR", Confidence: 0.6}

		for range defaultSNRMinObs {
			So(AssignCategorySurpriseSNR(&measurement, field, CategoryStochasticBalance), ShouldBeNil)
			measurement.Confidence = 0.6
		}

		Convey("It should spike on an unexpected category, not on repeated selections", func() {
			repeated := Measurement{Symbol: "BTC/EUR", Confidence: 0.6}
			So(AssignCategorySurpriseSNR(&repeated, field, CategoryStochasticBalance), ShouldBeNil)

			rare := Measurement{Symbol: "BTC/EUR", Confidence: 0.6}
			So(AssignCategorySurpriseSNR(&rare, field, CategoryAggressiveDrive), ShouldBeNil)

			So(rare.SNR, ShouldBeGreaterThan, repeated.SNR)
		})

		Convey("It should scale confidence by category stability", func() {
			stable := Measurement{Symbol: "ETH/EUR", Confidence: 0.8}

			for range 24 {
				So(AssignCategorySurpriseSNR(&stable, field, CategoryStochasticBalance), ShouldBeNil)
				stable.Confidence = 0.8
			}

			stableAgain := Measurement{Symbol: "ETH/EUR", Confidence: 0.8}
			So(AssignCategorySurpriseSNR(&stableAgain, field, CategoryStochasticBalance), ShouldBeNil)

			twitch := Measurement{Symbol: "XRP/EUR", Confidence: 0.8}

			for index := range 24 {
				category := CategoryStochasticBalance

				if index%2 == 1 {
					category = CategoryAggressiveDrive
				}

				So(AssignCategorySurpriseSNR(&twitch, field, category), ShouldBeNil)
				twitch.Confidence = 0.8
			}

			twitchAgain := Measurement{Symbol: "XRP/EUR", Confidence: 0.8}
			So(AssignCategorySurpriseSNR(&twitchAgain, field, CategoryStochasticBalance), ShouldBeNil)

			So(stableAgain.Confidence, ShouldBeGreaterThan, twitchAgain.Confidence)
			So(stableAgain.Confidence, ShouldBeLessThan, 0.8)
			So(twitchAgain.Confidence, ShouldBeGreaterThan, 0)
		})
	})
}

const defaultSNRMinObs = 12

func BenchmarkAssignCategorySurpriseSNR(b *testing.B) {
	field, err := NewCategorySurpriseField([]CategoryType{
		CategoryVolumeStarvation,
		CategoryStochasticBalance,
		CategoryHiddenAbsorption,
		CategoryAggressiveDrive,
	}, DefaultCategorySurpriseAlpha)

	if err != nil {
		b.Fatal(err)
	}

	measurement := Measurement{
		Symbol:     "BTC/EUR",
		Confidence: 0.6,
	}

	b.ReportAllocs()

	for b.Loop() {
		measurement.Confidence = 0.6

		if err := AssignCategorySurpriseSNR(
			&measurement,
			field,
			CategoryStochasticBalance,
		); err != nil {
			b.Fatal(err)
		}
	}
}
