package resonance

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
)

const testAlpha = 0.03

func TestMeasureForecastSkill(t *testing.T) {
	Convey("Given strictly prior forecasts scored against zero return", t, func() {
		state := newSymbolState(testAlpha)

		Convey("A tied forecast should retain the uncertain prior", func() {
			So(state.measureForecastSkill(0, 0), ShouldBeNil)
			So(state.skillEvidence, ShouldAlmostEqual, 0.5)
		})

		Convey("Repeated baseline wins should raise sample-aware confidence", func() {
			So(state.measureForecastSkill(0.01, 0.01), ShouldBeNil)
			So(state.skillEvidence, ShouldAlmostEqual, 0.75)
			So(state.measureForecastSkill(0, 0), ShouldBeNil)
			So(state.skillEvidence, ShouldAlmostEqual, 0.75)
			So(state.measureForecastSkill(0.02, 0.02), ShouldBeNil)
			So(state.skillEvidence, ShouldAlmostEqual, 0.875)
		})

		Convey("A forecast worse than zero should reduce confidence", func() {
			So(state.measureForecastSkill(-0.01, 0.01), ShouldBeNil)
			So(state.skillEvidence, ShouldAlmostEqual, 0.25)
		})
	})

	Convey("Given a non-finite forecast", t, func() {
		err := newSymbolState(testAlpha).measureForecastSkill(math.NaN(), 0)

		Convey("It should reject the observation", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestForecastConfidence(t *testing.T) {
	Convey("Given a return forecast without identified residual noise", t, func() {
		confidence, err := newSymbolState(testAlpha).forecastConfidence(
			learning.RLSOutput{Value: 0.01},
		)

		Convey("It should retain the symmetric chance prior", func() {
			So(err, ShouldBeNil)
			So(confidence, ShouldEqual, chanceForecastSkill)
		})
	})

	Convey("Given positive and negative posterior predictive returns", t, func() {
		state := newSymbolState(testAlpha)
		positive, err := state.forecastConfidence(learning.RLSOutput{
			Value:            0.01,
			Scale:            0.005,
			DegreesOfFreedom: 12,
			Ready:            true,
		})
		So(err, ShouldBeNil)
		negative, err := state.forecastConfidence(learning.RLSOutput{
			Value:            -0.01,
			Scale:            0.005,
			DegreesOfFreedom: 12,
			Ready:            true,
		})

		Convey("They should carry the same magnitude-sensitive direction probability", func() {
			So(err, ShouldBeNil)
			So(positive, ShouldBeGreaterThan, chanceForecastSkill)
			So(positive, ShouldBeLessThan, 1)
			So(negative, ShouldAlmostEqual, positive)
		})
	})

	Convey("Given an invalid predictive distribution", t, func() {
		_, err := newSymbolState(testAlpha).forecastConfidence(learning.RLSOutput{
			Value:            0.01,
			Scale:            0,
			DegreesOfFreedom: 12,
			Ready:            true,
		})

		Convey("It should reject the distribution", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a zero forecast carrying an invalid ready distribution", t, func() {
		_, err := newSymbolState(testAlpha).forecastConfidence(learning.RLSOutput{
			Value:            0,
			Scale:            0,
			DegreesOfFreedom: 12,
			Ready:            true,
		})

		Convey("It should reject the distribution before returning chance", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkMeasureForecastSkill(b *testing.B) {
	state := newSymbolState(testAlpha)

	for b.Loop() {
		if err := state.measureForecastSkill(0.01, 0.01); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkForecastConfidence(b *testing.B) {
	state := newSymbolState(testAlpha)
	forecast := learning.RLSOutput{
		Value:            0.01,
		Scale:            0.005,
		DegreesOfFreedom: 12,
		Ready:            true,
	}

	for b.Loop() {
		if _, err := state.forecastConfidence(forecast); err != nil {
			b.Fatal(err)
		}
	}
}
