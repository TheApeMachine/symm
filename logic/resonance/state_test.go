package resonance

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasureForecastSkill(t *testing.T) {
	Convey("Given strictly prior forecasts scored against zero return", t, func() {
		state := newSymbolState(restAlpha)

		Convey("A tied forecast should contribute neutral evidence", func() {
			So(state.measureForecastSkill(0, 0), ShouldBeNil)
			So(state.confidence, ShouldAlmostEqual, 0.5)
		})

		Convey("Repeated baseline wins should raise sample-aware confidence", func() {
			So(state.measureForecastSkill(0.01, 0.01), ShouldBeNil)
			So(state.confidence, ShouldAlmostEqual, 0.75)
			So(state.measureForecastSkill(0.02, 0.02), ShouldBeNil)
			So(state.confidence, ShouldAlmostEqual, 0.875)
		})

		Convey("A forecast worse than zero should reduce confidence", func() {
			So(state.measureForecastSkill(-0.01, 0.01), ShouldBeNil)
			So(state.confidence, ShouldAlmostEqual, 0.25)
		})
	})

	Convey("Given a non-finite forecast", t, func() {
		err := newSymbolState(restAlpha).measureForecastSkill(math.NaN(), 0)

		Convey("It should reject the observation", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkMeasureForecastSkill(b *testing.B) {
	state := newSymbolState(restAlpha)

	for b.Loop() {
		if err := state.measureForecastSkill(0.01, 0.01); err != nil {
			b.Fatal(err)
		}
	}
}
