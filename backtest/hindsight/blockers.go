package hindsight

import (
	"fmt"
	"strings"
)

func blockerCandidates(context SignalContext, leg Leg) []blockerCandidate {
	candidates := make([]blockerCandidate, 0)
	candidates = append(candidates, measurementBlockers(context, leg)...)
	candidates = append(candidates, admissionBlockers(context)...)
	candidates = append(candidates, downstreamBlockers(context)...)
	candidates = append(candidates, stateBlockers(context)...)
	return candidates
}

func admissionBlockers(context SignalContext) []blockerCandidate {
	candidates := make([]blockerCandidate, 0)
	priority := 10

	if !context.Opportunity {
		// When the system did not recognize a setup, policy failures are useful
		// supporting facts but not the root cause to tune first.
		priority = 20
	}

	if margin, exists := context.Alternatives[admissionDirectionMarginKey]; exists && margin < 0 {
		gap := -margin
		target, hasTarget := context.Alternatives[admissionDirectionTargetKey]
		explanation := fmt.Sprintf(
			"direction %.4f differed from the configured entry direction by %.4f",
			context.Direction,
			gap,
		)

		if hasTarget {
			explanation = fmt.Sprintf(
				"direction %.4f did not satisfy the configured entry direction %.4f",
				context.Direction,
				target,
			)
		}

		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:         "admission:direction",
				Category:    DiagnosisDirection,
				Label:       "directional agreement",
				Observed:    context.Direction,
				Target:      target,
				HasTarget:   hasTarget,
				Gap:         gap,
				Severity:    gap,
				Explanation: explanation,
			},
			priority: priority,
		})
	}

	candidates = append(candidates, minimumGateBlocker(
		context.Alternatives,
		admissionThesisMarginKey,
		"admission:thesis_score",
		"thesis score",
		context.ThesisScore,
		"trading.admission.minimum_thesis_score",
		priority+1,
	)...)
	candidates = append(candidates, minimumGateBlocker(
		context.Alternatives,
		admissionConfidenceMarginKey,
		"admission:confidence",
		"entry confidence",
		context.Confidence,
		"trading.admission.minimum_confidence",
		priority+2,
	)...)
	candidates = append(candidates, minimumGateBlocker(
		context.Alternatives,
		admissionSupportMarginKey,
		"admission:support",
		"thesis support",
		context.ThesisSupport,
		"trading.admission.minimum_support",
		priority+3,
	)...)

	if margin, exists := context.Alternatives[admissionContradictionMarginKey]; exists && margin < 0 {
		gap := -margin
		boundary := context.ThesisContradiction + margin
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:       "admission:contradiction",
				Category:  DiagnosisAdmission,
				Label:     "maximum contradiction",
				Source:    "trading.admission.maximum_contradiction",
				Observed:  context.ThesisContradiction,
				Target:    boundary,
				HasTarget: true,
				Gap:       gap,
				Severity:  gap,
				Explanation: fmt.Sprintf(
					"contradiction %.4f exceeded the maximum %.4f by %.4f",
					context.ThesisContradiction,
					boundary,
					gap,
				),
			},
			priority: priority + 4,
		})
	}

	// Current captures retain the policy's exact margins. The graph threshold
	// is only a legacy gate when those modern margins are absent.
	if !hasAdmissionEvidence(context.Alternatives) &&
		context.GraphScore < context.AdmissionThreshold {
		gap := context.AdmissionThreshold - context.GraphScore
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:       "admission:graph_score",
				Category:  DiagnosisAdmission,
				Label:     "graph evidence score",
				Source:    "trading.evidence.minimum_score",
				Observed:  context.GraphScore,
				Target:    context.AdmissionThreshold,
				HasTarget: true,
				Gap:       gap,
				Severity:  gap,
				Explanation: fmt.Sprintf(
					"graph score %.4f was %.4f below admission boundary %.4f",
					context.GraphScore,
					gap,
					context.AdmissionThreshold,
				),
			},
			priority: priority + 5,
		})
	}

	if reasonCategory(context) == DiagnosisAdmission && len(candidates) == 0 {
		reasonPriority := priority

		if context.Opportunity {
			// A textual admission mention without a retained failed margin is not
			// enough evidence to recommend changing a boundary. Keep the flagged
			// opportunity as the primary fact and show this reason underneath it.
			reasonPriority = 55
		}

		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:         "admission:revalidated",
				Category:    DiagnosisAdmission,
				Label:       "admission revalidation",
				Gap:         1,
				Severity:    1,
				Explanation: firstNonEmpty(context.Reason, context.Cause),
			},
			priority: reasonPriority,
		})
	}

	return candidates
}

func minimumGateBlocker(
	alternatives map[string]float64,
	marginKey string,
	key string,
	label string,
	observed float64,
	source string,
	priority int,
) []blockerCandidate {
	margin, exists := alternatives[marginKey]

	if !exists || margin >= 0 {
		return nil
	}

	gap := -margin
	boundary := observed - margin

	return []blockerCandidate{{
		Blocker: Blocker{
			Key:       key,
			Category:  DiagnosisAdmission,
			Label:     label,
			Source:    source,
			Observed:  observed,
			Target:    boundary,
			HasTarget: true,
			Gap:       gap,
			Severity:  gap,
			Explanation: fmt.Sprintf(
				"%s %.4f was %.4f below the required %.4f",
				label,
				observed,
				gap,
				boundary,
			),
		},
		priority: priority,
	}}
}

func measurementBlockers(context SignalContext, leg Leg) []blockerCandidate {
	candidates := make([]blockerCandidate, 0)
	upward := leg.SellPrice > leg.BuyPrice
	_, directionMarginRecorded := context.Alternatives[admissionDirectionMarginKey]

	if !directionMarginRecorded && upward && context.Direction < 0 {
		gap := abs(context.Direction)
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "evidence:direction_wrong",
				Category: DiagnosisDirection,
				Label:    "directional thesis",
				Observed: context.Direction,
				Gap:      gap,
				Severity: gap,
				Explanation: fmt.Sprintf(
					"thesis pointed the wrong way (direction %.4f, confidence %.4f)",
					context.Direction,
					context.ThesisConfidence,
				),
			},
			priority: 1,
		})
	}

	measurementPriority := 30

	if !context.Opportunity {
		measurementPriority = 2
	}

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
				Category:    DiagnosisMeasurement,
				Label:       "opposing measurement",
				Source:      measurementSource(name),
				Observed:    value,
				Gap:         abs(value),
				Severity:    abs(value),
				Explanation: fmt.Sprintf("%s scored %.4f against the realized move", name, value),
			},
			priority: measurementPriority,
		})
	}

	currentAudit := context.Alternatives != nil || strings.Contains(
		strings.ToLower(context.Reason),
		"no actable opportunity",
	)

	if !context.Opportunity && currentAudit {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "opportunity:not_classified",
				Category: DiagnosisMeasurement,
				Label:    "opportunity recognition",
				Gap:      leg.ProfitPct,
				Severity: leg.ProfitPct,
				Explanation: "no opportunity classifier fired before or during the move, " +
					"so the system had no typed setup to admit",
			},
			priority: 3,
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
					Explanation: "the opportunity cleared recorded admission but no entry occurred; " +
						"the missing stage is allocation, desk validation, or venue submission",
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
					firstNonEmpty(context.Type, "opportunity"),
					firstNonEmpty(context.Reason, context.Cause, "decided nothing"),
				),
			},
			priority: 50,
		})
	}

	if !context.PredictiveReady {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "predictive:not_ready",
				Category: DiagnosisPredictive,
				Label:    "predictive calibration",
				Gap:      1,
				Severity: 1,
				Explanation: firstNonEmpty(
					context.PredictiveStatus,
					"predictive coding had not resolved enough forward outcomes",
				) + "; this was informational unless another recorded stage used it as a veto",
			},
			priority: 60,
		})
	}

	return candidates
}
