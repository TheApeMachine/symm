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
			So(current[controlThesisScore], ShouldEqual, 0.5)
			So(current[controlConfidence], ShouldEqual, 0.0)
			So(current[controlSupport], ShouldAlmostEqual, 0.1)
			So(current[controlContradiction], ShouldAlmostEqual, 0.3)
			So(current[controlGraphThreshold], ShouldEqual, 0.0)
			So(current[controlCausalAlpha], ShouldEqual, 1.0)
			So(current[controlIterations], ShouldEqual, 1.0)
			So(current[controlExploration], ShouldEqual, 1.0)
		})
	})

	Convey("Given a confidence incumbent above the no-information boundary", t, func() {
		config := system.NewConfig()
		config.Planner.MinimumConfidence = 0.8
		config.Planner.Admission.MinimumConfidence = 0.8
		space, err := newControlSpace(config)

		Convey("It should leave room for the regulator to move in both directions", func() {
			So(err, ShouldBeNil)
			So(space.bounds[controlConfidence].minimum,
				ShouldEqual, system.UninformativeDirectionConfidence)
			current := space.current(config)
			So(current[controlConfidence], ShouldAlmostEqual, 0.6)

			lowerFound := false
			higherFound := false

			for _, candidate := range space.candidates(current, 1) {
				confidence := space.value(controlConfidence, candidate)
				lowerFound = lowerFound || confidence < config.Planner.MinimumConfidence
				higherFound = higherFound || confidence > config.Planner.MinimumConfidence
			}

			So(lowerFound, ShouldBeTrue)
			So(higherFound, ShouldBeTrue)
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
		exploratory := space.exploratory(current, 1, 0)

		Convey("It should generate shrinking feasible coordinate interventions", func() {
			So(len(candidates), ShouldBeGreaterThan, 1)
			So(exploratory, ShouldNotResemble, current)

			for _, candidate := range candidates {
				for _, value := range candidate {
					So(value, ShouldBeBetweenOrEqual, 0.0, 1.0)
				}

				iterations := space.value(controlIterations, candidate)
				So(iterations, ShouldEqual, float64(int(iterations)))
				So(space.normalize(controlIterations, iterations),
					ShouldEqual, candidate[controlIterations])
			}
		})
	})
}

func TestControlSpaceApply(t *testing.T) {
	Convey("Given the normalized lower endpoint of every configurable actuator", t, func() {
		config := system.NewConfig()
		legacyUtility := config.Planner.MinimumUtility
		space, err := newControlSpace(config)
		So(err, ShouldBeNil)

		err = space.apply(controlVector{}, config)

		Convey("It should publish the exact domain lower bounds", func() {
			So(err, ShouldBeNil)
			So(config.Planner.MaxAllocationFraction, ShouldEqual, 0.0)
			So(config.Planner.Admission.MinimumThesisScore, ShouldEqual, 0.0)
			So(config.Planner.Admission.MinimumConfidence,
				ShouldEqual, system.UninformativeDirectionConfidence)
			So(config.Planner.Admission.MinimumSupport, ShouldEqual, 0.0)
			So(config.Planner.Admission.MaximumContradiction, ShouldEqual, 0.0)
			So(config.Planner.MinimumConfidence,
				ShouldEqual, system.UninformativeDirectionConfidence)
			So(config.Planner.MinimumGraphScore, ShouldEqual, -1.0)
			So(config.Planner.MinimumUtility, ShouldEqual, legacyUtility)
			So(config.Planner.CausalAlpha, ShouldEqual, 0.0)
			So(config.Planner.MCTSIterations, ShouldEqual, 1)
			So(config.Planner.ExplorationConstant, ShouldEqual, 0.0)
		})
	})
}

