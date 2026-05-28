package price

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReturnModelForecast(t *testing.T) {
	Convey("Given fewer than the required settled samples", t, func() {
		model := NewReturnModel()

		for range MinForwardSamples - 1 {
			model.Observe("hawkes", "bullish", 0.8, 0.016)
		}

		Convey("It should refuse to forecast", func() {
			expected, tradable := model.Forecast("hawkes", "bullish", 0.5)

			So(expected, ShouldEqual, 0)
			So(tradable, ShouldBeFalse)
		})
	})

	Convey("Given a statistically positive settled bucket", t, func() {
		model := NewReturnModel()

		for range MinForwardSamples {
			model.Observe("hawkes", "bullish", 0.8, 0.016)
		}

		Convey("It should map confidence to expected forward return", func() {
			expected, tradable := model.Forecast("hawkes", "bullish", 0.5)

			So(tradable, ShouldBeTrue)
			So(expected, ShouldAlmostEqual, 0.01, 1e-9)
		})
	})

	Convey("Given a warmed bucket whose mean does not clear zero", t, func() {
		model := NewReturnModel()

		for index := range MinForwardSamples {
			realized := 0.01

			if index%2 == 0 {
				realized = -0.01
			}

			model.Observe("hawkes", "choppy", 0.8, realized)
		}

		Convey("It should remain non-tradable", func() {
			expected, tradable := model.Forecast("hawkes", "choppy", 0.5)

			So(expected, ShouldEqual, 0)
			So(tradable, ShouldBeFalse)
		})
	})
}

func BenchmarkReturnModelForecast(b *testing.B) {
	model := NewReturnModel()

	for range MinForwardSamples {
		model.Observe("hawkes", "bullish", 0.8, 0.016)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = model.Forecast("hawkes", "bullish", 0.5)
	}
}
