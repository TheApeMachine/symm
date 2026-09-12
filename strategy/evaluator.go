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
	prices      []float64
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

/* SetPrices configures factual market prices for the fragment. */
func (evaluator *FragmentEvaluator) SetPrices(prices []float64) {
	evaluator.prices = prices
}

/* Reset clears fragment-local boundary state. */
func (evaluator *FragmentEvaluator) Reset() {
	evaluator.anchorIndex = -1
	evaluator.surfaces = nil
	evaluator.prices = nil
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

	entryValue, hasEntry := frameOrSurfacePrice(evaluator, fragment, decisionIdx, true)

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

	maxRealizable := entryValue
	maxIdx := decisionIdx

	for idx := decisionIdx + 1; idx < len(fragment); idx++ {
		val, defined := frameOrSurfacePrice(evaluator, fragment, idx, false)

		if !defined {
			continue
		}

		if val > maxRealizable {
			maxRealizable = val
			maxIdx = idx
		}
	}

	move := (maxRealizable - entryValue) / entryValue

	if move <= roundTrip {
		outcome.Correctness = -1.0
		outcome.Reinforcement = -1.0

		return outcome, nil
	}

	betterEntryFound := false
	minEntryAhead := entryValue

	for idx := decisionIdx + 1; idx <= maxIdx; idx++ {
		val, defined := frameOrSurfacePrice(evaluator, fragment, idx, true)

		if defined && val > 0 && val < minEntryAhead {
			minEntryAhead = val
		}
	}

	if minEntryAhead < entryValue*(1.0-roundTrip) {
		betterEntryFound = true
	}

	if betterEntryFound {
		missedDiscount := (entryValue - minEntryAhead) / entryValue
		outcome.Correctness = -clamp(missedDiscount, 0.1, 1.0)
		outcome.Timing = clamp(minEntryAhead/entryValue, 0, 1)
		outcome.Reinforcement = outcome.Correctness

		return outcome, nil
	}

	margin := move - roundTrip
	outcome.Correctness = clamp(margin/roundTrip, 0.1, 1.0)

	minEntry := entryValue
	for idx := 0; idx <= maxIdx; idx++ {
		val, defined := frameOrSurfacePrice(evaluator, fragment, idx, true)

		if defined && val > 0 && val < minEntry {
			minEntry = val
		}
	}

	maxFeasibleMove := (maxRealizable - minEntry) / minEntry

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

	exitVal, hasExit := frameOrSurfacePrice(evaluator, fragment, decisionIdx, false)

	if !hasExit || exitVal <= 0 {
		return outcome, nil
	}

	entryVal, hasEntry := frameOrSurfacePrice(evaluator, fragment, entryIdx, true)

	if !hasEntry || entryVal <= 0 {
		entryVal = exitVal
	}

	if decisionIdx == len(fragment)-1 {
		if exitVal < entryVal {
			loss := (exitVal - entryVal) / entryVal
			outcome.Correctness = clamp(loss, -1.0, -0.1)
			outcome.Timing = 0.0
			outcome.Reinforcement = outcome.Correctness

			return outcome, nil
		}

		gain := (exitVal - entryVal) / entryVal
		outcome.Correctness = clamp(gain, 0.1, 1.0)
		outcome.Timing = 1.0
		outcome.Reinforcement = outcome.Correctness

		return outcome, nil
	}

	bestLater := exitVal
	worstLater := exitVal

	for idx := decisionIdx + 1; idx < len(fragment); idx++ {
		val, defined := frameOrSurfacePrice(evaluator, fragment, idx, false)

		if !defined {
			continue
		}

		if val > bestLater {
			bestLater = val
		}

		if val < worstLater {
			worstLater = val
		}
	}

	if bestLater > exitVal {
		missed := (bestLater - exitVal) / exitVal
		outcome.Correctness = -clamp(missed, 0.1, 1.0)
		outcome.Timing = clamp(exitVal/bestLater, 0, 1)
		outcome.Reinforcement = outcome.Correctness

		return outcome, nil
	}

	if worstLater < exitVal {
		protection := (exitVal - worstLater) / exitVal
		outcome.Correctness = clamp(protection, 0.1, 1.0)
		outcome.Timing = 1.0
		outcome.Reinforcement = outcome.Correctness

		return outcome, nil
	}

	outcome.Correctness = 0.0
	outcome.Timing = 1.0
	outcome.Reinforcement = 0.0

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
At terminal collapse or when position is underwater, WAIT is strongly punished.
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

	currentVal, hasCurrent := frameOrSurfacePrice(evaluator, fragment, decisionIdx, false)
	entryVal, hasEntry := frameOrSurfacePrice(evaluator, fragment, entryIdx, true)

	if hasCurrent && hasEntry && currentVal < entryVal {
		bestLater := currentVal

		for idx := decisionIdx + 1; idx < len(fragment); idx++ {
			val, defined := frameOrSurfacePrice(evaluator, fragment, idx, false)

			if defined && val > bestLater {
				bestLater = val
			}
		}

		if bestLater <= currentVal || bestLater <= entryVal {
			loss := (entryVal - currentVal) / entryVal
			outcome.Correctness = -clamp(math.Max(0.5, loss*2.0), 0.5, 1.0)
			outcome.Timing = 0.0
			outcome.Reinforcement = outcome.Correctness

			return outcome, nil
		}
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

func frameOrSurfacePrice(
	evaluator *FragmentEvaluator,
	fragment [][]*data.Measurement[float64],
	index int,
	isAsk bool,
) (float64, bool) {
	if evaluator != nil && evaluator.surfaces != nil && index >= 0 && index < len(evaluator.surfaces) && evaluator.surfaces[index] != nil {
		surf := evaluator.surfaces[index]

		if isAsk && surf.BestAsk != nil && surf.BestAsk.Float64() > 0 {
			return surf.BestAsk.Float64(), true
		}

		if !isAsk {
			if !surf.FullyExecutable {
				if surf.SellableQty != nil && surf.SellableQty.Sign() > 0 && surf.ExecutableQty != nil {
					ratio := surf.ExecutableQty.Div(surf.SellableQty).Float64()

					if surf.ExecutableVWAP != nil && surf.ExecutableVWAP.Float64() > 0 {
						return surf.ExecutableVWAP.Float64() * ratio, true
					}

					if surf.ExecutableValue != nil && surf.ExecutableValue.Float64() > 0 {
						return surf.ExecutableValue.Float64() * ratio, true
					}

					if surf.BestBid != nil && surf.BestBid.Float64() > 0 {
						return surf.BestBid.Float64() * ratio, true
					}
				}

				if surf.ExecutableValue != nil && surf.ExecutableValue.Float64() > 0 {
					return surf.ExecutableValue.Float64(), true
				}

				if surf.ExecutableVWAP != nil && surf.ExecutableVWAP.Float64() > 0 {
					return surf.ExecutableVWAP.Float64(), true
				}

				if surf.BestBid != nil && surf.BestBid.Float64() > 0 {
					return surf.BestBid.Float64(), true
				}

				return 0, false
			}

			if surf.ExecutableVWAP != nil && surf.ExecutableVWAP.Float64() > 0 {
				return surf.ExecutableVWAP.Float64(), true
			}

			if surf.ExecutableValue != nil && surf.ExecutableValue.Float64() > 0 {
				if surf.SellableQty != nil && surf.SellableQty.Sign() > 0 {
					return surf.ExecutableValue.Div(surf.SellableQty).Float64(), true
				}

				return surf.ExecutableValue.Float64(), true
			}

			if surf.BestBid != nil && surf.BestBid.Float64() > 0 {
				return surf.BestBid.Float64(), true
			}
		}
	}

	if evaluator != nil && evaluator.prices != nil && index >= 0 && index < len(evaluator.prices) && evaluator.prices[index] > 0 {
		return evaluator.prices[index], true
	}

	if index >= 0 && index < len(fragment) {
		return framePrice(fragment[index])
	}

	return 0, false
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
