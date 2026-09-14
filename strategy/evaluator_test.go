package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func makeTestFrames(count int) [][]*data.Measurement[float64] {
	frames := make([][]*data.Measurement[float64], count)

	for index := 0; index < count; index++ {
		measurement := data.NewMeasurement[float64]("cvd", nil)
		measurement.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: float64(index)}
		frames[index] = []*data.Measurement[float64]{measurement}
	}

	return frames
}

func TestFragmentEvaluator(t *testing.T) {
	Convey("Given a FragmentEvaluator configured for a positive excursion leg", t, func() {
		evaluator := NewFragmentEvaluator()
		evaluator.SetAnchorIndex(10)
		evaluator.SetExtremumIndex(20)
		frames := makeTestFrames(30)

		So(evaluator.AnchorIndex(), ShouldEqual, 10)
		So(evaluator.ExtremumIndex(), ShouldEqual, 20)

		Convey("When evaluating entry decisions", func() {
			Convey("Entering before or at anchor B captures the move and is reinforced", func() {
				outcome, err := evaluator.EvaluateEntry(frames, 10)
				So(err, ShouldBeNil)
				So(outcome.Action, ShouldEqual, ActionEnter)
				So(outcome.Correctness, ShouldEqual, 1.0)
				So(outcome.Timing, ShouldEqual, 1.0)
				So(outcome.Reinforcement, ShouldEqual, 1.0)
			})

			Convey("Entering during excursion climb captures a fractional portion", func() {
				outcome, err := evaluator.EvaluateEntry(frames, 15)
				So(err, ShouldBeNil)
				So(outcome.Correctness, ShouldEqual, 0.5)
				So(outcome.Timing, ShouldEqual, 0.5)
				So(outcome.Reinforcement, ShouldEqual, 0.25)
			})

			Convey("Entering after peak extremum C is penalized", func() {
				outcome, err := evaluator.EvaluateEntry(frames, 25)
				So(err, ShouldBeNil)
				So(outcome.Correctness, ShouldEqual, -1.0)
				So(outcome.Timing, ShouldEqual, 0.0)
				So(outcome.Reinforcement, ShouldEqual, -1.0)
			})
		})

		Convey("When evaluating exit decisions", func() {
			Convey("Exiting before anchor B is premature and penalized", func() {
				outcome, err := evaluator.EvaluateExit(frames, 5, 2)
				So(err, ShouldBeNil)
				So(outcome.Action, ShouldEqual, ActionExit)
				So(outcome.Correctness, ShouldEqual, -1.0)
				So(outcome.Reinforcement, ShouldEqual, -1.0)
			})

			Convey("Exiting during climb cuts winners short and is penalized", func() {
				outcome, err := evaluator.EvaluateExit(frames, 15, 10)
				So(err, ShouldBeNil)
				So(outcome.Correctness, ShouldBeLessThan, 0)
				So(outcome.Reinforcement, ShouldBeLessThan, 0)
			})

			Convey("Exiting at peak extremum C captures the full move", func() {
				outcome, err := evaluator.EvaluateExit(frames, 20, 10)
				So(err, ShouldBeNil)
				So(outcome.Correctness, ShouldEqual, 1.0)
				So(outcome.Timing, ShouldEqual, 1.0)
				So(outcome.Reinforcement, ShouldEqual, 1.0)
			})

			Convey("Exiting during retracement protects capital", func() {
				outcome, err := evaluator.EvaluateExit(frames, 22, 10)
				So(err, ShouldBeNil)
				So(outcome.Correctness, ShouldBeGreaterThan, 0)
				So(outcome.Reinforcement, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When evaluating wait / hold decisions", func() {
			Convey("Waiting while flat before anchor B is penalized", func() {
				outcome, err := evaluator.EvaluateWait(frames, 10, false, -1)
				So(err, ShouldBeNil)
				So(outcome.Action, ShouldEqual, ActionWait)
				So(outcome.Correctness, ShouldEqual, -1.0)
				So(outcome.Reinforcement, ShouldEqual, -1.0)
			})

			Convey("Waiting while holding during excursion climb is reinforced", func() {
				outcome, err := evaluator.EvaluateWait(frames, 15, true, 10)
				So(err, ShouldBeNil)
				So(outcome.Action, ShouldEqual, ActionHold)
				So(outcome.Correctness, ShouldEqual, 1.0)
				So(outcome.Timing, ShouldEqual, 1.0)
				So(outcome.Reinforcement, ShouldEqual, 1.0)
			})

			Convey("Waiting while holding past peak C into retracement is penalized", func() {
				outcome, err := evaluator.EvaluateWait(frames, 25, true, 10)
				So(err, ShouldBeNil)
				So(outcome.Action, ShouldEqual, ActionHold)
				So(outcome.Correctness, ShouldBeLessThan, 0)
			})
		})

		Convey("Reproducibility: grading the same trace twice produces identical grades (AT-20)", func() {
			first, errFirst := evaluator.EvaluateEntry(frames, 10)
			second, errSecond := evaluator.EvaluateEntry(frames, 10)
			So(errFirst, ShouldBeNil)
			So(errSecond, ShouldBeNil)
			So(first.Correctness, ShouldEqual, second.Correctness)
			So(first.Timing, ShouldEqual, second.Timing)
			So(first.Reinforcement, ShouldEqual, second.Reinforcement)
		})
	})

	Convey("Given a FragmentEvaluator configured for negative non-event tape", t, func() {
		evaluator := NewFragmentEvaluator()
		evaluator.SetAnchorIndex(-1)
		evaluator.SetExtremumIndex(-1)
		frames := makeTestFrames(20)

		Convey("Entering on negative tape is penalized", func() {
			outcome, err := evaluator.EvaluateEntry(frames, 5)
			So(err, ShouldBeNil)
			So(outcome.Correctness, ShouldEqual, -1.0)
			So(outcome.Reinforcement, ShouldEqual, -1.0)
		})

		Convey("Waiting while flat on negative tape is rewarded", func() {
			outcome, err := evaluator.EvaluateWait(frames, 5, false, -1)
			So(err, ShouldBeNil)
			So(outcome.Action, ShouldEqual, ActionWait)
			So(outcome.Correctness, ShouldEqual, 1.0)
			So(outcome.Reinforcement, ShouldEqual, 1.0)
		})

		Convey("Exiting while holding on negative tape preserves capital and is rewarded", func() {
			outcome, err := evaluator.EvaluateExit(frames, 5, 0)
			So(err, ShouldBeNil)
			So(outcome.Action, ShouldEqual, ActionExit)
			So(outcome.Correctness, ShouldEqual, 1.0)
			So(outcome.Reinforcement, ShouldEqual, 1.0)
		})

		Convey("Holding while holding on negative tape is penalized", func() {
			outcome, err := evaluator.EvaluateWait(frames, 5, true, 0)
			So(err, ShouldBeNil)
			So(outcome.Action, ShouldEqual, ActionHold)
			So(outcome.Correctness, ShouldEqual, -1.0)
			So(outcome.Reinforcement, ShouldEqual, -1.0)
		})
	})

	Convey("Given incomplete or missing landmark boundaries", t, func() {
		evaluator := NewFragmentEvaluator()
		evaluator.SetAnchorIndex(10)
		evaluator.SetExtremumIndex(5) // Invalid C <= B
		frames := makeTestFrames(20)

		Convey("EvaluateEntry returns an explicit error rather than a zero fallback (AT-21)", func() {
			_, err := evaluator.EvaluateEntry(frames, 10)
			So(err, ShouldNotBeNil)
		})
	})
}
