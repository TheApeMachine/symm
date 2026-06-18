package market

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func fluidMeasurement(
	symbol string,
	price float64,
	strength float64,
	confidence float64,
	observedAt time.Time,
) logic.Measurement {
	return logic.Measurement{
		Source:     logic.SourceFluid,
		Symbol:     symbol,
		Price:      price,
		Strength:   strength,
		Confidence: confidence,
		Position:   logic.PositionTypeLong,
		Category:   logic.CategoryLaminar,
		ObservedAt: observedAt,
	}
}

func TestStoryForwardFeedbackLoop(t *testing.T) {
	Convey("Given a story with forward feedback enabled", t, func() {
		viper.Set("market.story.measurement_max_age", 30*time.Second)
		viper.Set("market.story.forward_return_min_samples", 1)
		viper.Set("market.story.forward_return_significance_z", 0.5)
		defer viper.Reset()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		story := NewStory(ctx, pool)

		So(story, ShouldNotBeNil)

		anchorAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
		measurement := fluidMeasurement("ETH/USD", 100, 0.5, 0.8, anchorAt)

		So(story.Update(measurementArtifact(measurement)), ShouldBeNil)

		story.SettleForwardFeedback(
			anchorAt.Add(10*time.Second),
			func(string) (float64, bool) { return 101, true },
		)

		Convey("When the forward window has not elapsed", func() {
			feedback := story.FeedbackFor("ETH/USD", logic.SourceFluid)

			Convey("It should not settle a label yet", func() {
				So(feedback, ShouldBeNil)
			})
		})

		story.SettleForwardFeedback(
			anchorAt.Add(31*time.Second),
			func(string) (float64, bool) { return 101, true },
		)

		Convey("When price advances and the forward window elapses", func() {
			feedback := story.FeedbackFor("ETH/USD", logic.SourceFluid)
			calibrated := measurementBySource(story.Measurements(), "ETH/USD", logic.SourceFluid)

			Convey("It should record calibration and sharpen stored scoring", func() {
				So(feedback, ShouldNotBeNil)
				So(feedback.Samples, ShouldEqual, 1)
				So(feedback.Scale, ShouldAlmostEqual, 2.5, 0.01)
				So(calibrated.ExpectedMoveBps, ShouldAlmostEqual, 160, 0.01)
				So(calibrated.Strength, ShouldAlmostEqual, 1.25, 0.01)
				So(calibrated.Confidence, ShouldAlmostEqual, 0.4, 0.01)
			})
		})
	})
}

func TestStoryForwardFeedbackMarkMissRetainsLabel(t *testing.T) {
	Convey("Given a matured forward label without a mark price", t, func() {
		viper.Set("market.story.measurement_max_age", 30*time.Second)
		viper.Set("market.story.forward_return_min_samples", 1)
		viper.Set("market.story.forward_return_significance_z", 0.5)
		defer viper.Reset()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		story := NewStory(ctx, pool)
		anchorAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
		measurement := fluidMeasurement("ETH/USD", 100, 0.5, 0.8, anchorAt)

		So(story.Update(measurementArtifact(measurement)), ShouldBeNil)

		story.SettleForwardFeedback(
			anchorAt.Add(31*time.Second),
			func(string) (float64, bool) { return 0, false },
		)

		Convey("It should keep the label until a mark is available", func() {
			So(story.FeedbackFor("ETH/USD", logic.SourceFluid), ShouldBeNil)

			story.SettleForwardFeedback(
				anchorAt.Add(32*time.Second),
				func(string) (float64, bool) { return 101, true },
			)

			feedback := story.FeedbackFor("ETH/USD", logic.SourceFluid)

			So(feedback, ShouldNotBeNil)
			So(feedback.Samples, ShouldEqual, 1)
		})
	})
}

func TestStoryForwardFeedbackPerSource(t *testing.T) {
	Convey("Given two sources on one symbol", t, func() {
		viper.Set("market.story.measurement_max_age", 30*time.Second)
		viper.Set("market.story.forward_return_min_samples", 1)
		viper.Set("market.story.forward_return_significance_z", 0.5)
		defer viper.Reset()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		story := NewStory(ctx, pool)
		anchorAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

		fluid := fluidMeasurement("BTC/USD", 100, 0.4, 0.5, anchorAt)
		hawkes := logic.Measurement{
			Source:     logic.SourceHawkes,
			Symbol:     "BTC/USD",
			Price:      100,
			Strength:   0.2,
			Confidence: 0.5,
			Position:   logic.PositionTypeLong,
			Category:   logic.CategoryLaminar,
			ObservedAt: anchorAt,
		}

		So(story.Update(measurementArtifact(fluid)), ShouldBeNil)
		So(story.Update(measurementArtifact(hawkes)), ShouldBeNil)

		story.SettleForwardFeedback(
			anchorAt.Add(31*time.Second),
			func(string) (float64, bool) { return 100.5, true },
		)

		fluidFeedback := story.FeedbackFor("BTC/USD", logic.SourceFluid)
		hawkesFeedback := story.FeedbackFor("BTC/USD", logic.SourceHawkes)

		Convey("It should keep independent calibration per source", func() {
			So(fluidFeedback, ShouldNotBeNil)
			So(hawkesFeedback, ShouldNotBeNil)
			So(fluidFeedback.Samples, ShouldEqual, 1)
			So(hawkesFeedback.Samples, ShouldEqual, 1)
			So(fluidFeedback.Scale, ShouldNotEqual, hawkesFeedback.Scale)
		})
	})
}

func BenchmarkStorySettleForwardFeedback(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	story := NewStory(ctx, pool)
	anchorAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

	for index := 0; index < 32; index++ {
		measurement := fluidMeasurement(
			"ETH/USD",
			100+float64(index)*0.01,
			0.5,
			0.8,
			anchorAt.Add(time.Duration(index)*time.Second),
		)

		_ = story.Update(measurementArtifact(measurement))
	}

	marks := map[string]float64{"ETH/USD": 101.25}
	now := anchorAt.Add(31 * time.Second)

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		story.SettleForwardFeedback(now, func(symbol string) (float64, bool) {
			mark, ok := marks[symbol]

			return mark, ok
		})
	}
}

func measurementBySource(
	measurements []logic.Measurement,
	symbol string,
	source logic.SourceType,
) logic.Measurement {
	for _, measurement := range measurements {
		if measurement.Symbol == symbol && measurement.Source == source {
			return measurement
		}
	}

	return logic.Measurement{}
}

func storyMeasurementsBySource(
	story *Story,
	symbol string,
	source logic.SourceType,
) logic.Measurement {
	raw, ok := story.symbols.Load(symbol)

	if !ok {
		return logic.Measurement{}
	}

	sources := raw.(*sync.Map)
	value, ok := sources.Load(source)

	if !ok {
		return logic.Measurement{}
	}

	return value.(logic.Measurement)
}
