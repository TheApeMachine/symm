package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewTrendPerspective(t *testing.T) {
	convey.Convey("Given the organic-trend playbook", t, func() {
		trend := NewTrendPerspective()

		convey.Convey("When an authentic driver is confirmed by the tape", func() {
			measurements := []Measurement{
				measurement(CategoryEndogenousAlpha, 1.4),
				measurement(CategoryAggressiveDrive, 1.6),
			}

			convey.Convey("It authorizes entry", func() {
				action := trend.Decide(measurements, nil)
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When the driver is present but the tape does not confirm", func() {
			action := trend.Decide(
				[]Measurement{measurement(CategoryEndogenousAlpha, 1.4)},
				nil,
			)

			convey.Convey("It does not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})

		convey.Convey("When the tape drives but the move is not authentic (no gate)", func() {
			action := trend.Decide(
				[]Measurement{measurement(CategoryAggressiveDrive, 2.0)},
				nil,
			)

			convey.Convey("It does not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})

		convey.Convey("When holding and the book flips against the position", func() {
			measurements := []Measurement{
				measurement(CategoryEndogenousAlpha, 1.4),
				measurement(CategoryAggressiveDrive, 1.6),
				measurement(CategoryActiveReversal, 1.5),
			}

			convey.Convey("It trips the stop", func() {
				action := trend.Decide(measurements, []ObservationType{ObservationHolding})
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionStopLoss)
			})
		})

		convey.Convey("When holding and the aggressive flow exhausts", func() {
			measurements := []Measurement{
				measurement(CategoryEndogenousAlpha, 1.4),
				measurement(CategoryAggressiveDrive, 1.6),
				measurement(CategoryThermalExhaustion, 1.5),
			}

			convey.Convey("It harvests the profit", func() {
				action := trend.Decide(measurements, []ObservationType{ObservationHolding})
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionTakeProfit)
			})
		})

		convey.Convey("When no trend categories are present", func() {
			found := trend.Walk([]Measurement{measurement(CategoryLaminar, 2.0)})

			convey.Convey("The perspective is not active", func() {
				convey.So(found, convey.ShouldBeNil)
			})
		})
	})
}

func BenchmarkTrendPerspectiveWalk(b *testing.B) {
	trend := NewTrendPerspective()
	measurements := []Measurement{
		measurement(CategoryEndogenousAlpha, 1.4),
		measurement(CategoryAggressiveDrive, 1.6),
	}

	for b.Loop() {
		_ = trend.Decide(measurements, nil)
	}
}
