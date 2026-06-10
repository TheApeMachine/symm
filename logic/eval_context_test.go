package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestEvalContextRegimeAdjustedConfidenceBaseline(t *testing.T) {
	Convey("Given calm and turbulent fluid measurements", t, func() {
		viper.Set("trading.entry.confidence_baseline", 0.55)
		viper.Set("trading.entry.turbulence_confidence_scale", 0.30)

		calm := NewEvalContext([]Measurement{
			*NewMeasurement(
				SourceFluid,
				"BTC/EUR",
				0,
				0.1,
				0,
				0,
				0,
				CategoryLaminar,
				RegimeTypeNone,
				PositionTypeNone,
				0.7,
				0,
			),
		}, nil)

		turbulent := NewEvalContext([]Measurement{
			*NewMeasurement(
				SourceFluid,
				"BTC/EUR",
				0,
				0.9,
				0,
				0,
				0,
				CategoryTurbulent,
				RegimeTypeNone,
				PositionTypeNone,
				0.7,
				0,
			),
		}, nil)

		calmBaseline, calmErr := calm.Resolve("confidence.regime_adjusted_baseline")
		turbulentBaseline, turbulentErr := turbulent.Resolve("confidence.regime_adjusted_baseline")

		Convey("It should raise the confidence floor when turbulence is high", func() {
			So(calmErr, ShouldBeNil)
			So(turbulentErr, ShouldBeNil)
			So(calmBaseline, ShouldAlmostEqual, 0.58, 1e-9)
			So(turbulentBaseline, ShouldAlmostEqual, 0.82, 1e-9)
			So(turbulentBaseline, ShouldBeGreaterThan, calmBaseline)
		})
	})
}

func TestEvalContextResolveRequiresConfig(t *testing.T) {
	Convey("Given missing trading entry config", t, func() {
		viper.Set("trading.entry.confidence_baseline", 0.0)
		viper.Set("trading.entry.turbulence_confidence_scale", 0.0)

		evalContext := NewEvalContext(nil, nil)

		_, err := evalContext.Resolve("confidence.regime_adjusted_baseline")

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCategoryDynamicConfidence(t *testing.T) {
	Convey("Given a category subject with a dynamic confidence ref", t, func() {
		category := &Category{
			Type:          CategoryFrenzy,
			confidenceRef: "confidence.regime_adjusted_baseline",
		}
		evalContext := NewEvalContext([]Measurement{
			*NewMeasurement(
				SourceFluid,
				"BTC/EUR",
				0,
				0.9,
				0,
				0,
				0,
				CategoryTurbulent,
				RegimeTypeNone,
				PositionTypeNone,
				0.7,
				0,
			),
		}, nil)

		subject := NewSubject(
			SourceHawkes,
			SubjectCategory,
			category,
			nil,
			nil,
			0,
			0,
			0,
			0,
			0,
			0,
			0,
		)

		strong := *NewMeasurement(
			SourceHawkes,
			"BTC/EUR",
			0,
			0,
			0,
			0,
			0,
			CategoryFrenzy,
			RegimeTypeNone,
			PositionTypeNone,
			0.85,
			2.5,
		)
		weak := *NewMeasurement(
			SourceHawkes,
			"BTC/EUR",
			0,
			0,
			0,
			0,
			0,
			CategoryFrenzy,
			RegimeTypeNone,
			PositionTypeNone,
			0.60,
			2.5,
		)

		viper.Set("trading.entry.confidence_baseline", 0.55)
		viper.Set("trading.entry.turbulence_confidence_scale", 0.30)

		Convey("It should reject confidence below the regime-adjusted floor", func() {
			strongMatch, strongErr := subject.Evaluate(strong, evalContext)
			weakMatch, weakErr := subject.Evaluate(weak, evalContext)

			So(strongErr, ShouldBeNil)
			So(weakErr, ShouldBeNil)
			So(strongMatch, ShouldBeTrue)
			So(weakMatch, ShouldBeFalse)
		})
	})
}
