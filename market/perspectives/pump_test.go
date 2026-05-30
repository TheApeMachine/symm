package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func measurement(category CategoryType, snr float64) Measurement {
	return Measurement{Category: category, SNR: snr}
}

func TestNewPumpPerspectiveWalk(t *testing.T) {
	convey.Convey("Given a pump perspective tree", t, func() {
		pump := NewPumpPerspective()

		convey.Convey("When coiled compression clears the noise floor", func() {
			action := pump.Decide(
				[]Measurement{measurement(CategoryCoiledCompression, 1.2)},
				nil,
			)

			convey.Convey("It should authorize slow-pump entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When holding and faded exhaustion appears", func() {
			action := pump.DecideExit(
				[]Measurement{measurement(CategoryFadedExhaustion, 1.4)},
				[]ObservationType{ObservationHolding},
			)

			convey.Convey("It should take profit", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionTakeProfit)
			})
		})

		convey.Convey("When spoof trap and vertical ignition fire while held", func() {
			action := pump.DecideExit(
				[]Measurement{
					measurement(CategorySpoofTrap, 1.3),
					measurement(CategoryVerticalIgnition, 1.4),
				},
				[]ObservationType{ObservationHolding},
			)

			convey.Convey("It should close the long for a flip cue", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionShort)
			})
		})
	})
}

func BenchmarkPumpPerspectiveWalk(b *testing.B) {
	pump := NewPumpPerspective()
	measurements := []Measurement{
		measurement(CategoryCoiledCompression, 1.2),
		measurement(CategoryVerticalIgnition, 1.4),
	}

	for b.Loop() {
		_ = pump.Decide(measurements, nil)
	}
}
