package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewLeadLagPerspective(t *testing.T) {
	convey.Convey("Given the lead-lag playbook", t, func() {
		leadlag := NewLeadLagPerspective()

		convey.Convey("When breadth and inefficient lag align", func() {
			action := leadlag.Decide([]Measurement{
				measurement(CategoryDivergentMove, 1.2),
				measurement(CategoryInefficientLag, 1.3),
			}, nil)

			convey.Convey("It authorizes entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When holding and the follower has synchronized", func() {
			action := leadlag.DecideExit(
				[]Measurement{measurement(CategorySynchronizedDrift, 1.4)},
				[]ObservationType{ObservationHolding},
			)

			convey.Convey("It harvests the catch-up", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionTakeProfit)
			})
		})
	})
}

func BenchmarkLeadLagPerspectiveWalk(b *testing.B) {
	leadlag := NewLeadLagPerspective()
	measurements := []Measurement{
		measurement(CategoryDivergentMove, 1.2),
		measurement(CategoryInefficientLag, 1.3),
	}

	for b.Loop() {
		_ = leadlag.Decide(measurements, nil)
	}
}
