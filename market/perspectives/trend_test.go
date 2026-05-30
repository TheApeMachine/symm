package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func trendEntryMeasurements() []Measurement {
	return []Measurement{
		measurement(CategoryRiskOnSurge, 1.3),
		measurement(CategoryEndogenousAlpha, 1.4),
		measurement(CategoryFrenzy, 1.2),
		measurement(CategoryAggressiveDrive, 1.6),
	}
}

func TestNewTrendPerspective(t *testing.T) {
	convey.Convey("Given the organic-trend playbook", t, func() {
		trend := NewTrendPerspective()

		convey.Convey("When breadth, causal driver, timing, and tape align", func() {
			action := trend.Decide(trendEntryMeasurements(), nil)

			convey.Convey("It authorizes entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When toxic bluff is present", func() {
			measurements := append(trendEntryMeasurements(), measurement(CategoryToxicBluff, 1.5))
			action := trend.Decide(measurements, nil)

			convey.Convey("It denies entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionDeny)
			})
		})

		convey.Convey("When the driver is present but the tape does not confirm", func() {
			action := trend.Decide([]Measurement{
				measurement(CategoryRiskOnSurge, 1.3),
				measurement(CategoryEndogenousAlpha, 1.4),
				measurement(CategoryFrenzy, 1.2),
			}, nil)

			convey.Convey("It does not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})

		convey.Convey("When holding and the book flips without entry gates hot", func() {
			action := trend.DecideExit(
				[]Measurement{measurement(CategoryActiveReversal, 1.5)},
				[]ObservationType{ObservationHolding},
			)

			convey.Convey("It trips the stop", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionStopLoss)
			})
		})

		convey.Convey("When holding and flow exhausts", func() {
			action := trend.DecideExit(
				[]Measurement{measurement(CategoryThermalExhaustion, 1.5)},
				[]ObservationType{ObservationHolding},
			)

			convey.Convey("It harvests after soft-hold window in the trader", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionTakeProfit)
			})
		})
	})
}

func BenchmarkTrendPerspectiveWalk(b *testing.B) {
	trend := NewTrendPerspective()
	measurements := trendEntryMeasurements()

	for b.Loop() {
		_ = trend.Decide(measurements, nil)
	}
}
