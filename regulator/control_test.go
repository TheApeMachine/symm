package regulator

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestNewControlSpace(t *testing.T) {
	Convey("Given valid configured control domains", t, func() {
		config := system.NewConfig()
		space, err := newControlSpace(config)

		Convey("It should preserve every live actuator in normalized coordinates", func() {
			So(err, ShouldBeNil)
			current := space.current(config)
			So(current[controlAllocation], ShouldEqual, 1.0)
			So(current[controlConfidence], ShouldEqual, 0.0)
			So(current[controlCausalAlpha], ShouldEqual, 1.0)
			So(current[controlIterations], ShouldEqual, 1.0)
			So(current[controlExploration], ShouldEqual, 1.0)
			So(current[controlRelaxation], ShouldBeBetweenOrEqual, 0.0, 1.0)
		})
	})
}

func TestControlSpaceCandidates(t *testing.T) {
	Convey("Given an incumbent vector at several configured boundaries", t, func() {
		config := system.NewConfig()
		space, err := newControlSpace(config)
		So(err, ShouldBeNil)
		current := space.current(config)

		candidates := space.candidates(current, 1)
		exploratory := space.exploratory(current, 1)

		Convey("It should generate shrinking feasible coordinate interventions", func() {
			So(len(candidates), ShouldBeGreaterThan, 1)
			So(exploratory, ShouldNotResemble, current)

			for _, candidate := range candidates {
				for _, value := range candidate {
					So(value, ShouldBeBetweenOrEqual, 0.0, 1.0)
				}
			}
		})
	})
}

func TestControlSpaceApply(t *testing.T) {
	Convey("Given the normalized lower endpoint of every configurable actuator", t, func() {
		config := system.NewConfig()
		space, err := newControlSpace(config)
		So(err, ShouldBeNil)

		err = space.apply(controlVector{}, config)

		Convey("It should publish the exact domain lower bounds", func() {
			So(err, ShouldBeNil)
			So(config.Planner.MaxAllocationFraction, ShouldEqual, 0.0)
			So(config.Planner.MinimumConfidence,
				ShouldEqual, space.bounds[controlConfidence].minimum)
			So(config.Planner.CausalAlpha, ShouldEqual, 0.0)
			So(config.Planner.MCTSIterations, ShouldEqual, 1)
			So(config.Planner.ExplorationConstant, ShouldEqual, 0.0)
			So(config.Manifold.RelaxationSteps, ShouldEqual, config.Manifold.MinSteps)
		})
	})
}
