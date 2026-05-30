package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewScarcityPerspective(t *testing.T) {
	convey.Convey("Given the thin-market convexity playbook", t, func() {
		scarcity := NewScarcityPerspective()

		convey.Convey("When a scarce symbol ignites vertically", func() {
			measurements := []Measurement{
				measurement(CategoryExtremeScarcity, 1.4),
				measurement(CategoryVerticalIgnition, 1.6),
			}

			convey.Convey("It authorizes entry", func() {
				action := scarcity.Decide(measurements, nil)
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When a scarce symbol is coiled and loaded", func() {
			measurements := []Measurement{
				measurement(CategoryExtremeScarcity, 1.4),
				measurement(CategoryCoiledCompression, 1.2),
			}

			convey.Convey("It authorizes entry", func() {
				action := scarcity.Decide(measurements, nil)
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When the symbol is scarce but there is no ignition", func() {
			action := scarcity.Decide(
				[]Measurement{measurement(CategoryExtremeScarcity, 1.4)},
				nil,
			)

			convey.Convey("It does not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})

		convey.Convey("When there is ignition but the market is not scarce (no gate)", func() {
			action := scarcity.Decide(
				[]Measurement{measurement(CategoryVerticalIgnition, 2.0)},
				nil,
			)

			convey.Convey("It does not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})

		convey.Convey("When holding and the thin book reverses", func() {
			measurements := []Measurement{
				measurement(CategoryExtremeScarcity, 1.4),
				measurement(CategoryVerticalIgnition, 1.6),
				measurement(CategoryActiveReversal, 1.5),
			}

			convey.Convey("It trips the stop", func() {
				action := scarcity.Decide(measurements, []ObservationType{ObservationHolding})
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionStopLoss)
			})
		})

		convey.Convey("When holding and the leg fades", func() {
			measurements := []Measurement{
				measurement(CategoryExtremeScarcity, 1.4),
				measurement(CategoryVerticalIgnition, 1.6),
				measurement(CategoryFadedExhaustion, 1.5),
			}

			convey.Convey("It harvests before the book gaps back", func() {
				action := scarcity.Decide(measurements, []ObservationType{ObservationHolding})
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionTakeProfit)
			})
		})

		convey.Convey("When no scarcity categories are present", func() {
			found := scarcity.Walk([]Measurement{measurement(CategoryLaminar, 2.0)})

			convey.Convey("The perspective is not active", func() {
				convey.So(found, convey.ShouldBeNil)
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
