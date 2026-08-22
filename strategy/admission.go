package strategy

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

const (
	admissionAcceptedKey              = "admission:accepted"
	admissionDirectionMarginKey       = "admission:direction_margin"
	admissionThesisMarginKey          = "admission:thesis_score_margin"
	admissionConfidenceMarginKey      = "admission:confidence_margin"
	admissionSupportMarginKey         = "admission:support_margin"
	admissionContradictionMarginKey   = "admission:contradiction_margin"
	predictiveReadyEvidenceKey        = "evidence:predictive_ready"
	liquidityScoreKey                 = "evidence:liquidity_score"
	liquidityMassKey                  = "evidence:liquidity_mass"
	executionCoverageKey              = "execution:visible_coverage"
	executionFrictionKey              = "execution:friction_fraction"
	executionSpreadKey                = "execution:spread_fraction"
	executionImpactKey                = "execution:impact_fraction"
	candidateRankSumKey               = "ranking:ordinal_sum"
	candidateRankingDimensionCountKey = "ranking:dimensions"
)

func alternativesOf(decision *types.Decision) map[string]float64 {
	if decision.Alternatives == nil {
		decision.Alternatives = make(map[string]float64)
	}

	return decision.Alternatives
}

/*
applyAdmission is the sole strategy admission boundary. It records every
margin and every failed dimension. Predictive readiness and opportunity labels
remain visible evidence, but neither can veto a decision that clears the
operator's explicit policy.
*/
func applyAdmission(
	decision *types.Decision,
	policy types.AdmissionPolicy,
	graph *types.Graph,
) bool {
	if decision == nil {
		return false
	}

	alternatives := alternativesOf(decision)
	liquidity, mass := graphLiquidity(graph)
	alternatives[liquidityScoreKey] = liquidity
	alternatives[liquidityMassKey] = mass
	alternatives[predictiveReadyEvidenceKey] = 0

	if decision.PredictiveReady {
		alternatives[predictiveReadyEvidenceKey] = 1
	}

	alternatives[admissionDirectionMarginKey] = -math.Abs(
		decision.Direction - policy.RequiredDirection,
	)
	alternatives[admissionThesisMarginKey] =
		decision.ThesisScore - policy.MinimumThesisScore
	alternatives[admissionConfidenceMarginKey] =
		decision.Confidence - policy.MinimumConfidence
	alternatives[admissionSupportMarginKey] =
		decision.ThesisSupport - policy.MinimumSupport
	alternatives[admissionContradictionMarginKey] =
		policy.MaximumContradiction - decision.ThesisContradiction

	result := policy.Evaluate(*decision)

	if result.Accepted {
		alternatives[admissionAcceptedKey] = 1
		return true
	}

	alternatives[admissionAcceptedKey] = 0
	decision.Action = types.ActionNothing
	decision.Reason = "planner: admission rejected: " + result.Explanation()

	if explanation := directEvidenceExplanation(graph); explanation != "" {
		decision.Reason += "; " + explanation
	}

	if !decision.Opportunity {
		decision.Reason += "; no actable opportunity identified"
	}

	if !decision.PredictiveReady && decision.PredictiveStatus != "" {
		decision.Reason += "; predictive state is informational: " +
			decision.PredictiveStatus
	}

	return false
}

func configuredAdmission() (types.AdmissionPolicy, error) {
	config := system.Cfg.Snapshot()

	if config == nil || config.Planner == nil {
		return types.AdmissionPolicy{}, fmt.Errorf(
			"planner: admission configuration required",
		)
	}

	return config.Planner.Admission, nil
}

func admittedByCurrentPolicy(decision *types.Decision) bool {
	if decision == nil {
		return false
	}

	policy, err := configuredAdmission()

	if err != nil {
		return false
	}

	return policy.Evaluate(*decision).Accepted
}

/*
rankAdmissionCandidates applies equal-weight ordinal rank aggregation across
all requested quality dimensions plus graph and liquidity evidence. This avoids
inventing a multiplier that silently declares one score's units more important
than another's. Lower rank sums are better.
*/
func rankAdmissionCandidates(decisions []*types.Decision) {
	candidates := make([]*types.Decision, 0, len(decisions))

	for _, decision := range decisions {
		if decision != nil && decision.Action == types.ActionEnter {
			candidates = append(candidates, decision)
		}
	}

	if len(candidates) == 0 {
		return
	}

	dimensions := []func(*types.Decision) float64{
		func(decision *types.Decision) float64 { return decision.ThesisScore },
		func(decision *types.Decision) float64 { return decision.Confidence },
		func(decision *types.Decision) float64 { return decision.ThesisSupport },
		func(decision *types.Decision) float64 { return -decision.ThesisContradiction },
		func(decision *types.Decision) float64 {
			return alternativesOf(decision)[liquidityScoreKey]
		},
		func(decision *types.Decision) float64 {
			return alternativesOf(decision)[executionCoverageKey]
		},
		func(decision *types.Decision) float64 {
			return -alternativesOf(decision)[executionFrictionKey]
		},
		func(decision *types.Decision) float64 { return decision.GraphScore },
	}

	for _, decision := range candidates {
		alternatives := alternativesOf(decision)
		alternatives[candidateRankSumKey] = 0
		alternatives[candidateRankingDimensionCountKey] = float64(len(dimensions))
	}

	for _, dimension := range dimensions {
		ordered := slices.Clone(candidates)
		slices.SortStableFunc(ordered, func(left, right *types.Decision) int {
			leftValue := dimension(left)
			rightValue := dimension(right)

			switch {
			case leftValue > rightValue:
				return -1
			case leftValue < rightValue:
				return 1
			default:
				return strings.Compare(left.Symbol, right.Symbol)
			}
		})

		for start := 0; start < len(ordered); {
			end := start + 1
			value := dimension(ordered[start])

			for end < len(ordered) && dimension(ordered[end]) == value {
				end++
			}

			// Tied candidates receive the mean ordinal rank of the tied range.
			rank := float64(start+end-1) / 2

			for _, decision := range ordered[start:end] {
				alternativesOf(decision)[candidateRankSumKey] += rank
			}

			start = end
		}
	}
}
