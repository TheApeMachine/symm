package strategy

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ActionOutcome is the evaluator's judgment of one action the learner chose,
determined after the replay fragment has played out.

Correctness answers "was this action right?" given what subsequently happened.
Timing answers "how close was this to the best feasible time to act?"

Both are signed feedback: positive reinforces, negative inhibits. The learner
never sees these before choosing; they arrive only afterward.
*/
type ActionOutcome struct {
	Action      Action
	Correctness float64 // [-1, 1]
	Timing      float64 // [0, 1]: 1 = economically best feasible point
}

/*
FragmentEvaluator scores the learner's chosen action against the objective
market facts that followed it. It knows future facts because it is evaluating
completed historical replay. It must not leak those facts into the learner's
input before the action is chosen.

The evaluator uses frame values from the remaining fragment to determine
whether the learner's action was correct and how well-timed it was.
*/
type FragmentEvaluator struct {
	// feeRate is the round-trip fee fraction from the canonical economic owner.
	// When nil, evaluation returns an error rather than fabricating friction.
	feeRate *float64
}

/*
NewFragmentEvaluator constructs an evaluator with canonical fee information.
If feeRate is nil, the evaluator refuses to evaluate entry correctness.
*/
func NewFragmentEvaluator(feeRate *float64) *FragmentEvaluator {
	return &FragmentEvaluator{feeRate: feeRate}
}

/*
SetFeeRate configures the canonical fee rate for economic qualification.
*/
func (evaluator *FragmentEvaluator) SetFeeRate(rate float64) {
	evaluator.feeRate = &rate
}

/*
FeeRate returns the configured canonical fee rate, if set.
*/
func (evaluator *FragmentEvaluator) FeeRate() *float64 {
	return evaluator.feeRate
}

/*
EvaluateEntry scores an ENTER action chosen at decisionIdx within a fragment.
The fragment continues from decisionIdx+1 onward.

Correctness: Was entering justified by what subsequently became an upward-moving
leg whose realizable move cleared actual economic friction?

Timing: When entry was correct, how early within the valid entry window did the
learner act?
*/
func (evaluator *FragmentEvaluator) EvaluateEntry(
	fragment [][]*data.Measurement[float64],
	decisionIdx int,
) (ActionOutcome, error) {
	if evaluator.feeRate == nil {
		return ActionOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"evaluator: fee rate unavailable, cannot evaluate entry",
			nil,
		))
	}

	outcome := ActionOutcome{Action: ActionEnter}

	if decisionIdx < 0 || decisionIdx >= len(fragment) {
		return outcome, nil
	}

	entryValue, hasEntry := frameValue(fragment[decisionIdx])

	if !hasEntry || entryValue == 0 {
		return outcome, nil
	}

	// Find the maximum value reached after the decision point.
	maxValue := entryValue
	maxIdx := decisionIdx

	for idx := decisionIdx + 1; idx < len(fragment); idx++ {
		value, defined := frameValue(fragment[idx])

		if !defined {
			continue
		}

		if value > maxValue {
			maxValue = value
			maxIdx = idx
		}
	}

	// The realizable move as a fraction of entry.
	move := (maxValue - entryValue) / entryValue
	roundTrip := *evaluator.feeRate * 2

	if move <= roundTrip {
		// The move did not clear friction. ENTER was wrong here.
		outcome.Correctness = -1.0

		return outcome, nil
	}

	// Entry was correct: the move cleared friction.
	// Scale correctness by how much margin existed beyond friction.
	margin := move - roundTrip
	outcome.Correctness = clamp(margin/roundTrip, 0.1, 1.0)

	// Timing: how early in the valid window did the learner enter?
	// Earlier entry into a friction-clearing leg is better.
	remaining := len(fragment) - decisionIdx - 1
	legLength := maxIdx - decisionIdx

	if remaining > 0 && legLength > 0 {
		outcome.Timing = clamp(1.0-float64(legLength)/float64(remaining), 0, 1)
	}

	return outcome, nil
}

/*
EvaluateExit scores an EXIT action chosen at decisionIdx within a fragment
while the learner was holding a position entered at entryIdx.

Correctness: Was EXIT preferable to continuing to hold? Based on realizable
outcome, not proximity to the eventual chart maximum.

Timing: Was the exit made while sufficient value remained, before holding became
economically inferior?
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

	exitValue, hasExit := frameValue(fragment[decisionIdx])

	if !hasExit || exitValue == 0 {
		return outcome, nil
	}

	// What happened after the exit decision?
	endValue := exitValue

	if len(fragment) > decisionIdx+1 {
		if value, defined := frameValue(fragment[len(fragment)-1]); defined {
			endValue = value
		}
	}

	// If the price fell after exit, exit was correct.
	// If the price rose significantly after exit, exit was premature.
	postExitMove := (endValue - exitValue) / exitValue

	if postExitMove <= 0 {
		// Price fell or stayed flat after exit: good exit.
		outcome.Correctness = clamp(1.0+postExitMove*10, 0.5, 1.0)
	}

	if postExitMove > 0 {
		// Price continued rising: premature exit.
		// But don't punish too harshly — the exit might still have been
		// economically sound if liquidity was about to vanish.
		outcome.Correctness = clamp(-postExitMove*5, -1.0, 0.0)
	}

	// Timing: based on how much of the available value the learner captured.
	// Find peak value between entry and end of fragment.
	peakValue := exitValue

	for idx := entryIdx; idx < len(fragment); idx++ {
		value, defined := frameValue(fragment[idx])

		if defined && value > peakValue {
			peakValue = value
		}
	}

	entryValue, hasEntryValue := frameValue(fragment[entryIdx])

	if hasEntryValue && peakValue > entryValue {
		totalAvailable := peakValue - entryValue
		captured := exitValue - entryValue

		if totalAvailable > 0 {
			outcome.Timing = clamp(captured/totalAvailable, 0, 1)
		}
	}

	return outcome, nil
}

/*
EvaluateWait scores a WAIT action against what would have happened had the
learner taken the alternative action.

When flat (alternative is ENTER): If entering would have been profitable,
WAIT gets negative feedback. If entering would have been premature, WAIT
gets positive feedback.

When holding (alternative is EXIT): If exiting would have preserved value,
WAIT gets negative feedback. If holding was better, WAIT gets positive
feedback.
*/
func (evaluator *FragmentEvaluator) EvaluateWait(
	fragment [][]*data.Measurement[float64],
	decisionIdx int,
	holding bool,
	entryIdx int,
) (ActionOutcome, error) {
	outcome := ActionOutcome{Action: ActionWait}

	if !holding {
		// Flat: evaluate what ENTER would have done.
		enterOutcome, err := evaluator.EvaluateEntry(fragment, decisionIdx)

		if err != nil {
			return outcome, err
		}

		// WAIT is the inverse of ENTER: if entering was good, waiting was bad.
		outcome.Correctness = -enterOutcome.Correctness
		outcome.Timing = 1.0 - enterOutcome.Timing

		return outcome, nil
	}

	// Holding: evaluate what EXIT would have done.
	exitOutcome, err := evaluator.EvaluateExit(fragment, decisionIdx, entryIdx)

	if err != nil {
		return outcome, err
	}

	// WAIT is the inverse of EXIT: if exiting was good, waiting was bad.
	outcome.Correctness = -exitOutcome.Correctness
	outcome.Timing = 1.0 - exitOutcome.Timing

	return outcome, nil
}


func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}

	if value > high {
		return high
	}

	return value
}
