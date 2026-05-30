package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewLeadLagPerspective(t *testing.T) {
	convey.Convey("Given the anchor catch-up playbook", t, func() {
		leadlag := NewLeadLagPerspective()

		convey.Convey("When a follower lags the leader's move", func() {
			action := leadlag.Decide(
				[]Measurement{measurement(CategoryInefficientLag, 1.3)},
				nil,
			)

			convey.Convey("It authorizes entry on the catch-up", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When the lag is below the noise floor", func() {
			action := leadlag.Decide(
				[]Measurement{measurement(CategoryInefficientLag, 0.8)},
				nil,
			)

			convey.Convey("It does not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})

		convey.Convey("When holding and the book reverses", func() {
			measurements := []Measurement{
				measurement(CategoryInefficientLag, 1.3),
				measurement(CategoryActiveReversal, 1.4),
			}

			convey.Convey("It trips the stop", func() {
				action := leadlag.Decide(measurements, []ObservationType{ObservationHolding})
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionStopLoss)
			})
		})

		convey.Convey("When holding and the follower has caught up", func() {
			measurements := []Measurement{
				measurement(CategoryInefficientLag, 1.3),
				measurement(CategorySynchronizedDrift, 1.4),
			}

			convey.Convey("It harvests the position", func() {
				action := leadlag.Decide(measurements, []ObservationType{ObservationHolding})
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionTakeProfit)
			})
		})

		convey.Convey("When no lead-lag categories are present", func() {
			found := leadlag.Walk([]Measurement{measurement(CategoryLaminar, 2.0)})

			convey.Convey("The perspective is not active", func() {
				convey.So(found, convey.ShouldBeNil)
			})
		})
	})
}

func BenchmarkLeadLagPerspectiveWalk(b *testing.B) {
	leadlag := NewLeadLagPerspective()
	measurements := []Measurement{measurement(CategoryInefficientLag, 1.3)}

	for b.Loop() {
		_ = leadlag.Decide(measurements, nil)
	}
}
