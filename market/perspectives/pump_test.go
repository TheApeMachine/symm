package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func measurement(category CategoryType, snr float64) Measurement {
	return Measurement{Category: category, SNR: snr, Confidence: 0.8}
}

func TestNewPumpPerspectiveWalk(t *testing.T) {
	convey.Convey("Given a pump perspective tree", t, func() {
		pump := NewPumpPerspective()

		convey.Convey("When coiled compression is below the noise floor", func() {
			action := pump.Tree.Walk(
				[]Measurement{measurement(CategoryCoiledCompression, 0.8)},
				nil,
			)

			convey.Convey("It should not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})

		convey.Convey("When coiled compression clears the noise floor", func() {
			action := pump.Tree.Walk(
				[]Measurement{measurement(CategoryCoiledCompression, 1.2)},
				nil,
			)

			convey.Convey("It should authorize slow-pump entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When coiled compression and vertical ignition clear the noise floor", func() {
			action := pump.Tree.Walk(
				[]Measurement{
					measurement(CategoryCoiledCompression, 1.2),
					measurement(CategoryVerticalIgnition, 1.4),
				},
				nil,
			)

			convey.Convey("It should authorize flash-pump entry after ignition confirms", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When coiled compression is in position and still holding", func() {
			action := pump.Tree.Walk(
				[]Measurement{measurement(CategoryCoiledCompression, 1.2)},
				[]ObservationType{ObservationHasContinued, ObservationHolding},
			)

			convey.Convey("It should ratchet the stop with the spike", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionStopLoss)
			})
		})

		convey.Convey("When only spoof trap clears the noise floor", func() {
			action := pump.Tree.Walk(
				[]Measurement{measurement(CategorySpoofTrap, 1.3)},
				nil,
			)

			convey.Convey("It should enter early on the spoof signal", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When no pump categories are present", func() {
			found := pump.Walk([]Measurement{measurement(CategoryOrganicTrend, 2.0)})

			convey.Convey("It should not activate the perspective", func() {
				convey.So(found, convey.ShouldBeNil)
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
		_ = pump.Tree.Walk(measurements, nil)
	}
}
