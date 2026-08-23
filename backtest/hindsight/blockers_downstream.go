package hindsight

import (
	"fmt"
	"strings"
)

func downstreamBlockers(context SignalContext) []blockerCandidate {
	category := reasonCategory(context)

	switch category {
	case DiagnosisRegulator:
		return []blockerCandidate{{
			Blocker: Blocker{
				Key:         "regulator:entry_delayed",
				Category:    DiagnosisRegulator,
				Label:       "global regulator readiness",
				Gap:         1,
				Severity:    1,
				Explanation: firstNonEmpty(context.Reason, context.Cause),
			},
			priority: 15,
		}}
	case DiagnosisAllocation:
		return allocationBlockers(context)
	case DiagnosisExecution:
		return executionBlockers(context)
	default:
		return nil
	}
}

func allocationBlockers(context SignalContext) []blockerCandidate {
	reason := firstNonEmpty(context.Reason, context.Cause, "allocation did not admit the entry")
	candidates := make([]blockerCandidate, 0)
	lowerReason := strings.ToLower(reason)

	if context.SlotCapacity > 0 &&
		(strings.Contains(lowerReason, "slot") || strings.Contains(lowerReason, "capacity")) {
		required := context.OpenPositions + 1
		gap := max(0, required-context.SlotCapacity)
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:       "allocation:slot_capacity",
				Category:  DiagnosisAllocation,
				Label:     "position slot capacity",
				Source:    "trading.slots.total",
				Observed:  float64(context.SlotCapacity),
				Target:    float64(required),
				HasTarget: true,
				Gap:       float64(gap),
				Severity:  float64(gap),
				Explanation: fmt.Sprintf(
					"%d positions were already open against capacity %d; one additional slot was required",
					context.OpenPositions,
					context.SlotCapacity,
				),
			},
			priority: 20,
		})
	}

	candidates = append(candidates, blockerCandidate{
		Blocker: Blocker{
			Key:         "allocation:rejected",
			Category:    DiagnosisAllocation,
			Label:       "allocation or position capacity",
			Source:      context.AllocationClass,
			Gap:         1,
			Severity:    1,
			Explanation: reason,
		},
		priority: 21,
	})

	if context.AllocationHaircut > 0 {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "allocation:haircut",
				Category: DiagnosisAllocation,
				Label:    "allocation haircut",
				Source:   context.AllocationHaircutReason,
				Observed: context.AllocationHaircut,
				Gap:      context.AllocationHaircut,
				Severity: context.AllocationHaircut,
				Explanation: fmt.Sprintf(
					"allocation removed %.2f%% of the pre-risk notional: %s",
					context.AllocationHaircut*100,
					firstNonEmpty(context.AllocationHaircutReason, "no reason retained"),
				),
			},
			priority: 22,
		})
	}

	if !context.ReserveEligible && context.ReserveReason != "" {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:         "allocation:reserve_ineligible",
				Category:    DiagnosisAllocation,
				Label:       "reserve-lane qualification",
				Source:      "trading.slots.reserved",
				Gap:         1,
				Severity:    1,
				Explanation: context.ReserveReason,
			},
			priority: 23,
		})
	}

	return candidates
}

func executionBlockers(context SignalContext) []blockerCandidate {
	reason := firstNonEmpty(context.Reason, context.Cause, "entry did not reach the venue")
	candidates := make([]blockerCandidate, 0)

	if coverage, exists := context.Alternatives[executionCoverageKey]; exists && coverage < completeExecutionCoverage {
		gap := completeExecutionCoverage - coverage
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:       executionCoverageKey,
				Category:  DiagnosisExecution,
				Label:     "visible execution coverage",
				Observed:  coverage,
				Target:    completeExecutionCoverage,
				HasTarget: true,
				Gap:       gap,
				Severity:  gap,
				Explanation: fmt.Sprintf(
					"visible asks covered %.4f of the requested entry rather than the complete order",
					coverage,
				),
			},
			priority: 20,
		})
	}

	candidates = append(candidates, blockerCandidate{
		Blocker: Blocker{
			Key:         "execution:rejected",
			Category:    DiagnosisExecution,
			Label:       "execution feasibility",
			Gap:         1,
			Severity:    1,
			Explanation: reason,
		},
		priority: 21,
	})

	for _, evidence := range []struct {
		key   string
		label string
	}{
		{executionFrictionKey, "round-trip execution friction"},
		{executionSpreadKey, "entry spread"},
		{executionImpactKey, "visible book impact"},
	} {
		value, exists := context.Alternatives[evidence.key]

		if !exists || value <= 0 {
			continue
		}

		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:         evidence.key,
				Category:    DiagnosisExecution,
				Label:       evidence.label,
				Observed:    value,
				Gap:         value,
				Severity:    value,
				Explanation: fmt.Sprintf("%s contributed %.4f of entry price", evidence.label, value),
			},
			priority: 22,
		})
	}

	return candidates
}
