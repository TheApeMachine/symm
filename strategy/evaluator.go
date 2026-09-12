package strategy

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ActionOutcome is the evaluator's judgment of one action the learner chose,
determined after the replay fragment has played out.

Correctness answers "was this action right?" given what subsequently happened.
Timing answers "how close was this to the best feasible time to act?"
Reinforcement is the combined signed feedback written to the cognition trie:
positive reinforces, negative inhibits.
*/
type ActionOutcome struct {
	Action        Action
	Correctness   float64 // [-1, 1]
	Timing        float64 // [0, 1]: 1 = economically best feasible point
	Reinforcement float64 // [-1, 1]: combined reinforcement signal
}

/*
FragmentEvaluator scores the learner's chosen action directly against the
objective ground truth excursion geometry discovered on the tape fragment:
Anchor (B) -> Extremum (C) -> Retracement.

The green bar in Hindsight has already proven that this excursion leg crossed
friction (fees, etc.). The evaluator therefore requires no embedded prices,
no simulated fees, and no shadow economy models.
*/
type FragmentEvaluator struct {
	anchorIndex   int
	extremumIndex int
}

/* NewFragmentEvaluator constructs an excursion geometry evaluator. */
func NewFragmentEvaluator(_ ...any) *FragmentEvaluator {
	return &FragmentEvaluator{
		anchorIndex:   -1,
		extremumIndex: -1,
	}
}

/* SetAnchorIndex sets the objective event boundary B for the current fragment. */
func (evaluator *FragmentEvaluator) SetAnchorIndex(anchorIndex int) {
	evaluator.anchorIndex = anchorIndex
}

/* SetExtremumIndex sets the objective event boundary C (peak) for the current fragment. */
func (evaluator *FragmentEvaluator) SetExtremumIndex(extremumIndex int) {
	evaluator.extremumIndex = extremumIndex
}

/* AnchorIndex returns the configured anchor boundary B. */
func (evaluator *FragmentEvaluator) AnchorIndex() int {
	return evaluator.anchorIndex
}

/* ExtremumIndex returns the configured extremum boundary C. */
func (evaluator *FragmentEvaluator) ExtremumIndex() int {
	return evaluator.extremumIndex
}

/* Reset clears fragment-local boundary state. */
func (evaluator *FragmentEvaluator) Reset() {
	evaluator.anchorIndex = -1
	evaluator.extremumIndex = -1
}

/*
EvaluateEntry scores an ENTER action chosen at decisionIdx within a fragment.
Judged directly against the ground-truth excursion leg [Anchor B, Extremum C].
*/
func (evaluator *FragmentEvaluator) EvaluateEntry(
	fragment [][]*data.Measurement[float64],
	decisionIdx int,
) (ActionOutcome, error) {
	outcome := ActionOutcome{Action: ActionEnter}

	if decisionIdx < 0 || decisionIdx >= len(fragment) {
		return outcome, nil
	}

	anchorIdx := evaluator.anchorIndex
	extremumIdx := evaluator.extremumIndex

	if anchorIdx < 0 || extremumIdx <= anchorIdx {
		return ActionOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"evaluator: valid excursion boundaries [Anchor B, Extremum C] required",
			nil,
		))
	}

	// 1. At or before anchor: entering before or at launch point captures the move
	if decisionIdx <= anchorIdx {
		outcome.Correctness = 1.0
		outcome.Timing = 1.0

		if anchorIdx > 0 {
			outcome.Timing = float64(decisionIdx) / float64(anchorIdx)
		}

		outcome.Reinforcement = outcome.Correctness * math.Max(0.1, outcome.Timing)

		return outcome, nil
	}

	// 2. During excursion climb towards extremum C: captures remaining portion
	if decisionIdx <= extremumIdx {
		span := float64(extremumIdx - anchorIdx)
		remaining := float64(extremumIdx - decisionIdx)
		ratio := remaining / span

		outcome.Correctness = ratio
		outcome.Timing = ratio
		outcome.Reinforcement = outcome.Correctness * outcome.Timing

		return outcome, nil
	}

	// 3. After extremum peak: entering late into retracement / dump
	outcome.Correctness = -1.0
	outcome.Timing = 0.0
	outcome.Reinforcement = -1.0

	return outcome, nil
}

/*
EvaluateExit scores an EXIT action chosen at decisionIdx while holding position entered at entryIdx.
Judged against peak extremum C and subsequent retracement.
*/
func (evaluator *FragmentEvaluator) EvaluateExit(
	fragment [][]*data.Measurement[float64],
	decisionIdx int,
	entryIdx int,
) (ActionOutcome, error) {
	outcome := ActionOutcome{Action: ActionExit}

	if decisionIdx < 0 || decisionIdx >= len(fragment) || entryIdx < 0 {
		return outcome, nil
	}

	anchorIdx := evaluator.anchorIndex
	extremumIdx := evaluator.extremumIndex

	if anchorIdx < 0 || extremumIdx <= anchorIdx {
		return outcome, nil
	}

	// 1. Exiting before anchor B: premature exit before the leg begins
	if decisionIdx < anchorIdx {
		outcome.Correctness = -1.0
		outcome.Timing = 0.0
		outcome.Reinforcement = -1.0

		return outcome, nil
	}

	// 2. Exiting during excursion before peak C: cutting winners short
	if decisionIdx < extremumIdx {
		span := float64(extremumIdx - anchorIdx)
		remaining := float64(extremumIdx - decisionIdx)
		ratio := remaining / span

		outcome.Correctness = -ratio
		outcome.Timing = 1.0 - ratio
		outcome.Reinforcement = outcome.Correctness * (1.0 - outcome.Timing)

		return outcome, nil
	}

	// 3. Exiting at peak C: captured full excursion
	if decisionIdx == extremumIdx {
		outcome.Correctness = 1.0
		outcome.Timing = 1.0
		outcome.Reinforcement = 1.0

		return outcome, nil
	}

	// 4. Exiting after peak during retracement: protects capital against further drawdown
	aftermathSpan := float64(len(fragment) - 1 - extremumIdx)

	if aftermathSpan <= 0 {
		outcome.Correctness = 0.5
		outcome.Timing = 1.0
		outcome.Reinforcement = 0.5

		return outcome, nil
	}

	distance := float64(decisionIdx - extremumIdx)
	protection := 1.0 - (distance / aftermathSpan)

	outcome.Correctness = math.Max(0.1, protection)
	outcome.Timing = math.Max(0.1, protection)
	outcome.Reinforcement = outcome.Correctness * outcome.Timing

	return outcome, nil
}

/*
EvaluateWait scores a WAIT action against what would have happened had the
alternative action been taken.
*/
func (evaluator *FragmentEvaluator) EvaluateWait(
	fragment [][]*data.Measurement[float64],
	decisionIdx int,
	holding bool,
	entryIdx int,
) (ActionOutcome, error) {
	outcome := ActionOutcome{Action: ActionWait}

	if decisionIdx < 0 || decisionIdx >= len(fragment) {
		return outcome, nil
	}

	anchorIdx := evaluator.anchorIndex
	extremumIdx := evaluator.extremumIndex

	if anchorIdx < 0 || extremumIdx <= anchorIdx {
		return outcome, nil
	}

	if !holding {
		enterOutcome, err := evaluator.EvaluateEntry(fragment, decisionIdx)

		if err != nil {
			return outcome, err
		}

		outcome.Correctness = -enterOutcome.Correctness
		outcome.Timing = 1.0 - enterOutcome.Timing
		outcome.Reinforcement = -enterOutcome.Reinforcement

		return outcome, nil
	}

	// When holding:
	// Climbing towards peak C: holding lets profits run
	if decisionIdx < extremumIdx {
		outcome.Correctness = 1.0
		outcome.Timing = 1.0
		outcome.Reinforcement = 1.0

		return outcome, nil
	}

	// At or past peak C: holding round-trips gains into retracement
	exitOutcome, err := evaluator.EvaluateExit(fragment, decisionIdx, entryIdx)

	if err != nil {
		return outcome, err
	}

	outcome.Correctness = -exitOutcome.Correctness
	outcome.Timing = 0.0
	outcome.Reinforcement = outcome.Correctness

	return outcome, nil
}
