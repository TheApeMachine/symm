package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewScarcityPerspective(t *testing.T) {
	convey.Convey("Given the scarcity playbook", t, func() {
		scarcity := NewScarcityPerspective()

		convey.Convey("When scarcity and vertical ignition align", func() {
			action := scarcity.Decide([]Measurement{
				measurement(CategoryExtremeScarcity, 1.4),
				measurement(CategoryVerticalIgnition, 1.6),
			}, nil)

			convey.Convey("It authorizes entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When holding and reversal fires without scarcity hot", func() {
			action := scarcity.DecideExit(
				[]Measurement{measurement(CategoryActiveReversal, 1.5)},
				[]ObservationType{ObservationHolding},
			)

			convey.Convey("It stops out", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionStopLoss)
			})
		})
	})
}

func BenchmarkScarcityPerspectiveWalk(b *testing.B) {
	scarcity := NewScarcityPerspective()
	measurements := []Measurement{
		measurement(CategoryExtremeScarcity, 1.4),
		measurement(CategoryVerticalIgnition, 1.6),
	}

	for b.Loop() {
		_ = scarcity.Decide(measurements, nil)
	}
}
