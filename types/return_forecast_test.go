package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResonanceReturnForecastKey(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ResonanceReturnForecastKey is read", func() {
			Convey("it is return_forecast", func() {
				So(ResonanceReturnForecastKey, ShouldEqual, "return_forecast")
			})
		})
	})
}

func TestResonanceReturnForecast(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a live call is constructed", func() {
			f := ResonanceReturnForecast{
				Horizon:          8,
				CandidateCall:    1,
				Call:             1,
				StableCall:       1,
				Held:             false,
				SwitchConfidence: 0.75,
				SwitchThreshold:  0.6,
			}

			Convey("fields retain the actionable direction", func() {
				So(f.Horizon, ShouldEqual, 8)
				So(f.CandidateCall, ShouldEqual, 1)
				So(f.Call, ShouldEqual, 1)
				So(f.StableCall, ShouldEqual, 1)
				So(f.Held, ShouldBeFalse)
				So(f.SwitchConfidence, ShouldEqual, 0.75)
				So(f.SwitchThreshold, ShouldEqual, 0.6)
			})
		})

		Convey("When a held state is constructed", func() {
			f := ResonanceReturnForecast{
				Horizon:          4,
				CandidateCall:    -1,
				Call:             0,
				StableCall:       1,
				Held:             true,
				SwitchConfidence: 0.4,
				SwitchThreshold:  0.6,
			}

			Convey("Call is zero while StableCall retains the side", func() {
				So(f.Horizon, ShouldEqual, 4)
				So(f.CandidateCall, ShouldEqual, -1)
				So(f.Call, ShouldEqual, 0)
				So(f.StableCall, ShouldEqual, 1)
				So(f.Held, ShouldBeTrue)
				So(f.SwitchConfidence, ShouldEqual, 0.4)
				So(f.SwitchThreshold, ShouldEqual, 0.6)
			})
		})
	})
}
