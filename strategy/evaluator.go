package strategy

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
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
FragmentEvaluator scores the learner's chosen action against the objective
market facts that followed it. It knows future facts because it is evaluating
completed historical replay. It must not leak those facts into the learner's
input before the action is chosen.
*/
type FragmentEvaluator struct {
	feeRate     *float64
	price       *broker.Price
	anchorIndex int
}

/*
NewFragmentEvaluator constructs an evaluator with canonical fee information.
If feeRate and price are nil, the evaluator refuses to evaluate entry correctness.
*/
func NewFragmentEvaluator(feeRate *float64, price ...*broker.Price) *FragmentEvaluator {
	var prc *broker.Price

	if len(price) > 0 {
		prc = price[0]
	}

	return &FragmentEvaluator{
		feeRate:     feeRate,
		price:       prc,
		anchorIndex: -1,
	}
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
SetPrice configures the canonical broker.Price owner.
*/
func (evaluator *FragmentEvaluator) SetPrice(price *broker.Price) {
	evaluator.price = price
}

/*
Price returns the configured canonical broker.Price owner.
*/
func (evaluator *FragmentEvaluator) Price() *broker.Price {
	return evaluator.price
}

/*
SetAnchorIndex sets the objective event boundary B for the current fragment.
*/
func (evaluator *FragmentEvaluator) SetAnchorIndex(anchorIndex int) {
	evaluator.anchorIndex = anchorIndex
}

/*
Reset clears fragment-local boundary state.
*/
func (evaluator *FragmentEvaluator) Reset() {
	evaluator.anchorIndex = -1
}

/*
EvaluateEntry scores an ENTER action chosen at decisionIdx within a fragment.
The fragment continues from decisionIdx+1 onward.

Correctness: Was entering justified by what subsequently became an upward-moving
leg whose realizable move cleared actual economic friction?

Timing: Closeness to the precursor maturation boundary (anchor B). Premature
entry during early precursor buildup is penalized, teaching the learner to WAIT.
*/
func (evaluator *FragmentEvaluator) EvaluateEntry(
	fragment [][]*data.Measurement[float64],
	decisionIdx int,
) (ActionOutcome, error) {
	if evaluator.feeRate == nil && evaluator.price == nil {
		return ActionOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"evaluator: fee rate and price unavailable, cannot evaluate entry",
			nil,
		))
	}

	outcome := ActionOutcome{Action: ActionEnter}

	if decisionIdx < 0 || decisionIdx >= len(fragment) {
		return outcome, nil
	}

	entryValue, hasEntry := framePrice(fragment[decisionIdx])

	if !hasEntry || entryValue == 0 {
		return outcome, nil
	}

	maxValue := entryValue
	maxIdx := decisionIdx

	for idx := decisionIdx + 1; idx < len(fragment); idx++ {
		value, defined := framePrice(fragment[idx])

		if !defined {
			continue
		}

		if value > maxValue {
			maxValue = value
			maxIdx = idx
		}
	}

	move := (maxValue - entryValue) / entryValue
	roundTrip := 0.0

	if evaluator.feeRate != nil {
		roundTrip = *evaluator.feeRate * 2.0
	}

	if roundTrip <= 0 && evaluator.price != nil {
		symbol := ""

		if len(fragment[decisionIdx]) > 0 {
			symbol = fragment[decisionIdx][0].Label
		}

		fee := evaluator.price.FeeIfAvailable(symbol)

		if fee != nil && fee.Fee != nil {
			roundTrip = fee.Fee.Float64() * 0.02
		}
	}

	if roundTrip <= 0 {
		return ActionOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"evaluator: round trip fee unavailable for entry evaluation",
			nil,
		))
	}

	if move <= roundTrip {
		outcome.Correctness = -1.0
		outcome.Reinforcement = -1.0

		return outcome, nil
	}

	margin := move - roundTrip
	outcome.Correctness = clamp(margin/roundTrip, 0.1, 1.0)

	if evaluator.anchorIndex >= 0 {
		if decisionIdx <= evaluator.anchorIndex {
			precursorSpan := float64(evaluator.anchorIndex)
			outcome.Timing = clamp(1.0-(float64(evaluator.anchorIndex-decisionIdx)/math.Max(1, precursorSpan)), 0, 1)
		}

		if decisionIdx > evaluator.anchorIndex {
			legSpan := float64(len(fragment) - evaluator.anchorIndex)
			outcome.Timing = clamp(1.0-(float64(decisionIdx-evaluator.anchorIndex)/math.Max(1, legSpan)), 0, 1)
		}
	}

	if evaluator.anchorIndex < 0 {
		remaining := len(fragment) - decisionIdx - 1
		legLength := maxIdx - decisionIdx

		if remaining > 0 && legLength > 0 {
			outcome.Timing = clamp(1.0-float64(legLength)/float64(remaining), 0, 1)
		}
	}

	if outcome.Timing >= 0.5 {
		outcome.Reinforcement = outcome.Correctness * outcome.Timing
	}

	if outcome.Timing < 0.5 {
		outcome.Reinforcement = outcome.Correctness * (2.0*outcome.Timing - 1.0)
	}

	return outcome, nil
}

/*
EvaluateExit scores an EXIT action chosen at decisionIdx within a fragment
while the learner was holding a position entered at entryIdx.

Correctness: Was EXIT preferable to continuing to hold? Based on realizable
outcome against executable bid liquidity, not proximity to the chart maximum.

Timing: Was the exit made before value or liquidity deteriorated?
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

	exitValue, hasExit := framePrice(fragment[decisionIdx])

	if !hasExit || exitValue == 0 {
		return outcome, nil
	}

	endValue := exitValue

	if len(fragment) > decisionIdx+1 {
		if value, defined := framePrice(fragment[len(fragment)-1]); defined {
			endValue = value
		}
	}

	postExitMove := (endValue - exitValue) / exitValue

	if postExitMove <= 0 {
		outcome.Correctness = clamp(1.0+postExitMove*10.0, 0.5, 1.0)
	}

	if postExitMove > 0 {
		outcome.Correctness = clamp(-postExitMove*5.0, -1.0, 0.0)
	}

	peakValue := exitValue

	for idx := entryIdx; idx < len(fragment); idx++ {
		value, defined := framePrice(fragment[idx])

		if defined && value > peakValue {
			peakValue = value
		}
	}

	entryValue, hasEntryValue := framePrice(fragment[entryIdx])

	if hasEntryValue && peakValue > entryValue {
		totalAvailable := peakValue - entryValue
		captured := exitValue - entryValue

		if totalAvailable > 0 {
			outcome.Timing = clamp(captured/totalAvailable, 0, 1)
		}
	}

	if outcome.Correctness > 0 {
		outcome.Reinforcement = outcome.Correctness * math.Max(0.1, outcome.Timing)
	}

	if outcome.Correctness <= 0 {
		outcome.Reinforcement = outcome.Correctness
	}

	return outcome, nil
}

/*
EvaluateWait scores a WAIT action against what would have happened had the
learner taken the alternative action.

When flat (alternative is ENTER): If entering would have been premature,
WAIT gets positive reinforcement. If entering was mature, WAIT gets penalized.

When holding (alternative is EXIT): If exiting was correct (price fell afterward),
WAIT gets negative feedback. If holding captured continued gains, WAIT gets positive feedback.
*/
func (evaluator *FragmentEvaluator) EvaluateWait(
	fragment [][]*data.Measurement[float64],
	decisionIdx int,
	holding bool,
	entryIdx int,
) (ActionOutcome, error) {
	outcome := ActionOutcome{Action: ActionWait}

	if !holding {
		enterOutcome, err := evaluator.EvaluateEntry(fragment, decisionIdx)

		if err != nil {
			return outcome, err
		}

		outcome.Correctness = -enterOutcome.Correctness
		outcome.Timing = 1.0 - enterOutcome.Timing

		if enterOutcome.Timing < 0.5 {
			outcome.Reinforcement = clamp(1.0-2.0*enterOutcome.Timing, 0.1, 1.0)
		}

		if enterOutcome.Timing >= 0.5 {
			outcome.Reinforcement = -enterOutcome.Reinforcement
		}

		return outcome, nil
	}

	exitOutcome, err := evaluator.EvaluateExit(fragment, decisionIdx, entryIdx)

	if err != nil {
		return outcome, err
	}

	outcome.Correctness = -exitOutcome.Correctness
	outcome.Timing = 1.0 - exitOutcome.Timing
	outcome.Reinforcement = -exitOutcome.Reinforcement

	return outcome, nil
}

/* framePrice extracts genuine price data from a frame. */
func framePrice(frame []*data.Measurement[float64]) (float64, bool) {
	for _, measurement := range frame {
		if measurement == nil || len(measurement.Metrics) == 0 {
			continue
		}

		if measurement.Source == "price" || measurement.Source == "ticker" || measurement.Source == "sentiment" {
			for _, key := range []string{"last", "close", "raw", "price", "value"} {
				if metric, ok := measurement.Metrics[key]; ok && metric.Raw > 0 {
					return metric.Raw, true
				}
			}
		}

		if measurement.Label == "price" || measurement.Label == "last" || measurement.Label == "close" {
			for _, metric := range measurement.Metrics {
				if metric.Raw > 0 {
					return metric.Raw, true
				}
			}
		}
	}

	for _, measurement := range frame {
		if measurement == nil {
			continue
		}

		if metric, ok := measurement.Metrics["raw"]; ok && metric.Raw > 0 {
			return metric.Raw, true
		}

		if metric, ok := measurement.Metrics["price"]; ok && metric.Raw > 0 {
			return metric.Raw, true
		}
	}

	return 0, false
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
