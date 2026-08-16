package regulator

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestNewOptimizer(t *testing.T) {
	Convey("Given configured predictive coding and control bounds", t, func() {
		model, err := newOptimizer(system.NewConfig())

		Convey("It should construct a supervised temporal manifold", func() {
			So(err, ShouldBeNil)
			So(model, ShouldNotBeNil)
			So(model.coder, ShouldNotBeNil)
			So(model.pending, ShouldBeNil)
			So(model.resolved, ShouldEqual, 0)
		})
	})
}

func TestOptimizerUpdate(t *testing.T) {
	Convey("Given inactivity under restrictive admission controls", t, func() {
		model, err := newOptimizer(system.NewConfig())
		So(err, ShouldBeNil)
		model.current[controlAllocation] = 0
		model.current[controlConfidence] = 1
		model.current[controlGraphThreshold] = 1

		result, err := model.update(0, 0, false, markContext{})

		Convey("It should make the next policy maximally permissive and funded", func() {
			So(err, ShouldBeNil)
			So(result.controls[controlAllocation], ShouldEqual, 1.0)
			So(result.controls[controlConfidence], ShouldEqual, 0.0)
			So(result.controls[controlGraphThreshold], ShouldEqual, 0.0)
		})
	})

	Convey("Given a multi-leg sequence of subsequent account outcomes", t, func() {
		model, err := newOptimizer(system.NewConfig())
		So(err, ShouldBeNil)

		baseline, baselineErr := model.update(0, 0, false, markContext{})
		loss, lossErr := model.update(-0.05, -0.05, true, markContext{})
		recovery, recoveryErr := model.update(0.02, -0.03, true, markContext{})

		Convey("It should resolve each pending control exactly one outcome later", func() {
			So(baselineErr, ShouldBeNil)
			So(lossErr, ShouldBeNil)
			So(recoveryErr, ShouldBeNil)
			So(baseline.controls, ShouldResemble, model.baseline)
			So(loss.exploring, ShouldBeTrue)
			So(model.resolved, ShouldEqual, 2)
			So(model.pending, ShouldHaveLength, regulatorContextCount+controlCount)
			So(recovery.surprise, ShouldBeGreaterThanOrEqualTo, 0.0)
		})
	})

	Convey("Given outcomes that consistently improve as allocation is reduced", t, func() {
		model, err := newOptimizer(system.NewConfig())
		So(err, ShouldBeNil)
		_, err = model.update(0, 0, false, markContext{})
		So(err, ShouldBeNil)
		var result optimizationResult
		bestAllocation := 1.0
		optimizedCount := 0

		for range controlCount*controlCount*controlCount + 1 {
			appliedAllocation := model.current[controlAllocation]
			outcome := (1 - appliedAllocation) / float64(controlCount)
			result, err = model.update(outcome, 0, appliedAllocation > 0, markContext{})

			if err != nil {
				break
			}

			if !result.exploring {
				optimizedCount++
				bestAllocation = min(
					bestAllocation,
					result.controls[controlAllocation],
				)
			}
		}

		Convey("It should graduate from interventions to posterior candidate search", func() {
			So(err, ShouldBeNil)
			So(result.skillReady, ShouldBeTrue)
			So(result.skill, ShouldBeGreaterThan, 1.0)
			So(optimizedCount, ShouldBeGreaterThan, 0)
			So(bestAllocation, ShouldBeLessThan, 1.0)
		})
	})
}

func TestCandidateScoreBetter(t *testing.T) {
	Convey("Given the regulator's ordered outcome classes", t, func() {
		lossWithActivity := candidateScore{
			losing: true, returnFloor: -0.01, activityFloor: 1,
		}
		flatWithoutActivity := candidateScore{
			inactive: true, returnFloor: 0, activityFloor: 0,
		}
		activeNonLoss := candidateScore{
			returnFloor: 0, activityFloor: 0.5,
		}
		profitableActive := candidateScore{
			returnFloor: 0.01, activityFloor: 0.5,
		}

		Convey("A non-losing inactive outcome should outrank an active loss", func() {
			So(flatWithoutActivity.Better(lossWithActivity), ShouldBeTrue)
		})

		Convey("An active non-loss should outrank an inactive non-loss", func() {
			So(activeNonLoss.Better(flatWithoutActivity), ShouldBeTrue)
		})

		Convey("Profit should decide between equal loss and activity classes", func() {
			So(profitableActive.Better(activeNonLoss), ShouldBeTrue)
		})
	})
}
