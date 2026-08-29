package hindsight

import (
	"fmt"
	"strings"
)

func blockerCandidates(context SignalContext, leg Leg) []blockerCandidate {
	candidates := make([]blockerCandidate, 0)

	// Pipeline ordering: detection is upstream of valuation, which is upstream
	// of selection, which is upstream of execution. The earliest stage that
	// actually failed owns the regret.
	if !context.Opportunity {
		candidates = append(candidates, detectionBlockers(context, leg)...)
		candidates = append(candidates, measurementBlockers(context, leg)...)
		candidates = append(candidates, downstreamBlockers(context)...)
		candidates = append(candidates, stateBlockers(context)...)

		return candidates
	}

	candidates = append(candidates, valuationBlockers(context, leg)...)
	candidates = append(candidates, downstreamBlockers(context)...)
	candidates = append(candidates, stateBlockers(context)...)

	if len(candidates) == 0 || candidates[0].Category != DiagnosisValuation {
		candidates = append(candidates, selectionBlockers(context)...)
	}

	return candidates
}

/*
detectionBlockers records when the opportunity was never classified.
*/
func detectionBlockers(context SignalContext, leg Leg) []blockerCandidate {
	return []blockerCandidate{{
		Blocker: Blocker{
			Key:      "detection:not_classified",
			Category: DiagnosisDetection,
			Label:    "opportunity not detected",
			Gap:      leg.ProfitPct,
			Severity: 1,
			Explanation: "the opportunity type was never classified, so the system " +
				"had no typed setup to admit",
		},
		priority: 1,
	}}
}

/*
valuationBlockers records when economic consequence was not estimable: valuation
not attempted, not available, or causation unidentified. It only fires when the
opportunity was detected — valuation is downstream of detection.
*/
func valuationBlockers(context SignalContext, leg Leg) []blockerCandidate {
	candidates := make([]blockerCandidate, 0)

	if !context.ValuationAttempted {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "valuation:not_attempted",
				Category: DiagnosisValuation,
				Label:    "valuation not attempted",
				Gap:      leg.ProfitPct,
				Severity: 1,
				Explanation: "no valuation was attempted for this opportunity, so economic " +
					"consequence was not estimable at the decision point",
			},
			priority: 1,
		})
	} else if !context.ValuationAvailable {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "valuation:not_available",
				Category: DiagnosisValuation,
				Label:    "valuation unavailable",
				Gap:      leg.ProfitPct,
				Severity: 1,
				Explanation: firstNonEmpty(
					context.ValuationStatus,
					"valuation was attempted but no economic consequence was available",
				),
			},
			priority: 1,
		})
	} else if context.CausalIdentification == "" {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "valuation:causation_unidentified",
				Category: DiagnosisValuation,
				Label:    "causal identification absent",
				Gap:      leg.ProfitPct,
				Severity: 1,
				Explanation: "valuation did not identify a causal coordinate, so the " +
					"consequence it claimed could not be attributed to this opportunity",
			},
			priority: 2,
		})
	}

	return candidates
}

/*
selectionBlockers records what MCTS actually compared and selected: no utility,
no recommended action, or a CASH/WAIT preference.
*/
func selectionBlockers(context SignalContext) []blockerCandidate {
	candidates := make([]blockerCandidate, 0)

	if !context.UtilityAvailable {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "selection:no_utility",
				Category: DiagnosisSelection,
				Label:    "selection utility unavailable",
				Gap:      1,
				Severity: 1,
				Explanation: "no selection utility was recorded, so MCTS had no scored " +
					"alternative to compare",
			},
			priority: 20,
		})

		return candidates
	}

	recommended := context.MCTS.RecommendedAction

	if recommended == "" {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:         "selection:no_recommendation",
				Category:    DiagnosisSelection,
				Label:       "no recommended action",
				Gap:         1,
				Severity:    1,
				Explanation: "the search ran but recorded no recommended action",
			},
			priority: 20,
		})

		return candidates
	}

	if recommended == "nothing" || recommended == "wait" || recommended == "cash" {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "selection:preferred_cash",
				Category: DiagnosisSelection,
				Label:    "CASH/WAIT preferred",
				Gap:      1,
				Severity: 1,
				Explanation: fmt.Sprintf(
					"MCTS selected %s over entering this opportunity",
					recommended,
				),
			},
			priority: 20,
		})
	}

	return candidates
}

func measurementBlockers(context SignalContext, leg Leg) []blockerCandidate {
	candidates := make([]blockerCandidate, 0)
	upward := leg.SellPrice > leg.BuyPrice

	for name, value := range context.Alternatives {
		if !strings.HasPrefix(name, "meas:") {
			continue
		}

		if upward && value >= 0 {
			continue
		}

		if !upward && value <= 0 {
			continue
		}

		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:         name,
				Category:    DiagnosisValuation,
				Label:       "opposing measurement",
				Source:      measurementSource(name),
				Observed:    value,
				Gap:         abs(value),
				Severity:    abs(value),
				Explanation: fmt.Sprintf("%s scored %.4f against the realized move", name, value),
			},
			priority: 30,
		})
	}

	return candidates
}

func stateBlockers(context SignalContext) []blockerCandidate {
	candidates := make([]blockerCandidate, 0)

	if context.Opportunity {
		accepted, exists := context.Alternatives[admissionAcceptedKey]

		if exists && accepted > 0 {
			candidates = append(candidates, blockerCandidate{
				Blocker: Blocker{
					Key:      "funnel:admitted_without_entry",
					Category: DiagnosisFollowThrough,
					Label:    "admitted decision follow-through",
					Gap:      1,
					Severity: 1,
					Explanation: "the opportunity cleared recorded admission but no entry " +
						"occurred; the missing stage is allocation, desk validation, or venue submission",
				},
				priority: 40,
			})
		}

		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "opportunity:flagged_without_entry",
				Category: DiagnosisFollowThrough,
				Label:    "flagged opportunity follow-through",
				Gap:      1,
				Severity: 1,
				Explanation: fmt.Sprintf(
					"flagged as %s but no entry — %s",
					firstNonEmpty(context.OpportunityType, "opportunity"),
					firstNonEmpty(context.Reason, context.Cause, "decided nothing"),
				),
			},
			priority: 50,
		})
	}

	return candidates
}
