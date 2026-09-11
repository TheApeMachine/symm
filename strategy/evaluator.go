package strategy

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
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
	surfaces    []*types.ExecutionSurface
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

/* SetFeeRate configures a fee rate for economic qualification. */
func (evaluator *FragmentEvaluator) SetFeeRate(rate float64) {
	evaluator.feeRate = &rate
}

/* FeeRate returns the configured fee rate, if set. */
func (evaluator *FragmentEvaluator) FeeRate() *float64 {
	return evaluator.feeRate
}

/* SetPrice configures the canonical broker.Price owner. */
func (evaluator *FragmentEvaluator) SetPrice(price *broker.Price) {
	evaluator.price = price
}

/* Price returns the configured canonical broker.Price owner. */
func (evaluator *FragmentEvaluator) Price() *broker.Price {
	return evaluator.price
}

/* SetAnchorIndex sets the objective event boundary B for the current fragment. */
func (evaluator *FragmentEvaluator) SetAnchorIndex(anchorIndex int) {
	evaluator.anchorIndex = anchorIndex
}

/* SetSurfaces configures captured execution surfaces for the fragment. */
func (evaluator *FragmentEvaluator) SetSurfaces(surfaces []*types.ExecutionSurface) {
	evaluator.surfaces = surfaces
}

/* Reset clears fragment-local boundary state. */
func (evaluator *FragmentEvaluator) Reset() {
	evaluator.anchorIndex = -1
	evaluator.surfaces = nil
}

/*
EvaluateEntry scores an ENTER action chosen at decisionIdx within a fragment.
The fragment continues from decisionIdx+1 onward.

Correctness: Was entering justified by what subsequently became an upward-moving
leg whose realizable move cleared authoritative economic friction?

Timing: Fraction of feasible excursion move captured relative to the best feasible
entry price available during precursor development.
*/
func (evaluator *FragmentEvaluator) EvaluateEntry(
	fragment [][]*data.Measurement[float64],
	decisionIdx int,
) (ActionOutcome, error) {
	outcome := ActionOutcome{Action: ActionEnter}

	if decisionIdx < 0 || decisionIdx >= len(fragment) {
		return outcome, nil
	}

	entryValue, hasEntry := framePrice(fragment[decisionIdx])

	if !hasEntry || entryValue <= 0 {
		return ActionOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"evaluator: genuine market price unavailable in frame",
			nil,
		))
	}

	symbol := ""
	for _, m := range fragment[decisionIdx] {
		if m != nil && m.Label != "" && m.Label != "price" && m.Label != "last" && m.Label != "close" {
			symbol = m.Label
			break
		}
	}

	roundTrip := 0.0

	if evaluator.price != nil && symbol != "" {
		fee := evaluator.price.FeeIfAvailable(symbol)
		if fee != nil && fee.Fee != nil {
			roundTrip = fee.Fee.Float64() * 0.02
		}
	}

	if roundTrip <= 0 && evaluator.feeRate != nil {
		roundTrip = *evaluator.feeRate * 2.0
	}

	if roundTrip <= 0 {
		return ActionOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"evaluator: fee rate and price unavailable, cannot evaluate entry",
			nil,
		))
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

	if move <= roundTrip {
		outcome.Correctness = -1.0
		outcome.Reinforcement = -1.0

		return outcome, nil
	}

	margin := move - roundTrip
	outcome.Correctness = clamp(margin/roundTrip, 0.1, 1.0)

	// Economic timing: evaluate entry relative to the best feasible entry price
	minEntry := entryValue
	for idx := 0; idx <= maxIdx; idx++ {
		if val, defined := framePrice(fragment[idx]); defined && val > 0 && val < minEntry {
			minEntry = val
		}
	}

	maxFeasibleMove := (maxValue - minEntry) / minEntry
	if maxFeasibleMove > 0 {
		outcome.Timing = clamp(move/maxFeasibleMove, 0, 1)
	}

	outcome.Reinforcement = outcome.Correctness * math.Max(0.1, outcome.Timing)

	return outcome, nil
}

/*
EvaluateExit scores an EXIT action chosen at decisionIdx within a fragment
while the learner was holding a position entered at entryIdx.

Correctness: Was EXIT preferable to continuing to hold? Judged against realizable
liquidation proceeds on executable bid liquidity when surfaces are available,
or genuine price paths otherwise. Exiting before bid evaporation preserves realizable proceeds.

Timing: Fraction of peak realizable proceeds captured above entry basis.
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

	// 1. Authoritative evaluation against captured execution surfaces
	if evaluator.surfaces != nil && decisionIdx < len(evaluator.surfaces) && evaluator.surfaces[decisionIdx] != nil {
		currSurface := evaluator.surfaces[decisionIdx]
		if currSurface.ExecutableValue != nil && currSurface.ExecutableValue.Float64() > 0 {
			exitRealizable := currSurface.ExecutableValue.Float64()
			worstLater := exitRealizable
			bestLater := exitRealizable

			for idx := decisionIdx + 1; idx < len(evaluator.surfaces); idx++ {
				surf := evaluator.surfaces[idx]
				if surf == nil || surf.ExecutableValue == nil {
					continue
				}
				val := surf.ExecutableValue.Float64()
				if val < worstLater {
					worstLater = val
				}
				if val > bestLater {
					bestLater = val
				}
			}

			// If subsequent liquidity evaporated or price collapsed below exit realization:
			// EXIT successfully protected realizable liquidation capital before the pull.
			if worstLater < exitRealizable {
				protection := (exitRealizable - worstLater) / exitRealizable
				outcome.Correctness = clamp(protection, 0.1, 1.0)
				outcome.Timing = 1.0
				outcome.Reinforcement = outcome.Correctness
				return outcome, nil
			}

			// If bids remained deep and realizable proceeds continued to rise:
			// EXIT was premature, holding would have realized more proceeds.
			missed := (bestLater - exitRealizable) / exitRealizable
			outcome.Correctness = -clamp(missed, 0.1, 1.0)
			outcome.Timing = clamp(exitRealizable/bestLater, 0, 1)
			outcome.Reinforcement = outcome.Correctness
			return outcome, nil
		}
	}

	// 2. Fallback to genuine frame prices when execution surfaces are not provided
	exitValue, hasExit := framePrice(fragment[decisionIdx])

	if !hasExit || exitValue <= 0 {
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
		outcome.Correctness = clamp(-postExitMove, 0.1, 1.0)
	}

	if postExitMove > 0 {
		outcome.Correctness = -clamp(postExitMove, 0.1, 1.0)
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

When flat (alternative is ENTER): If entering would have been profitable,
WAIT gets negative feedback (missed opportunity). If entering would have lost,
WAIT gets positive feedback (avoided loss).

When holding (alternative is EXIT): If exiting was correct (liquidity evaporated or price fell),
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
		outcome.Reinforcement = -enterOutcome.Reinforcement

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

/* framePrice extracts genuine market price data from an authoritative price measurement. */
func framePrice(frame []*data.Measurement[float64]) (float64, bool) {
	for _, measurement := range frame {
		if measurement == nil || len(measurement.Metrics) == 0 {
			continue
		}

		if measurement.Source == "price" || measurement.Source == "ticker" || measurement.Source == "trade" {
			for _, key := range []string{"last", "close", "price"} {
				if metric, ok := measurement.Metrics[key]; ok && metric.Raw > 0 {
					return metric.Raw, true
				}
			}
		}

		if measurement.Label == "price" || measurement.Label == "last" || measurement.Label == "close" {
			for _, key := range []string{"last", "close", "price", "raw"} {
				if metric, ok := measurement.Metrics[key]; ok && metric.Raw > 0 {
					return metric.Raw, true
				}
			}
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
