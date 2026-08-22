package strategy

import (
	"testing"

	"github.com/theapemachine/symm/types"
)

func strategyAdmissionPolicy() types.AdmissionPolicy {
	return types.AdmissionPolicy{
		RequiredDirection:    1,
		MinimumThesisScore:   0.7,
		MinimumConfidence:    0.7,
		MinimumSupport:       1,
		MaximumContradiction: 0.3,
	}
}

func TestApplyAdmissionDoesNotUsePredictiveReadinessAsVeto(t *testing.T) {
	decision := types.NewDecision(types.ActionNothing, "TEST/USD")
	decision.Direction = 1
	decision.ThesisScore = 0.8
	decision.Confidence = 0.8
	decision.ThesisSupport = 1.2
	decision.ThesisContradiction = 0.1
	decision.PredictiveReady = false
	decision.PredictiveStatus = "task skill is still calibrating"

	if !applyAdmission(decision, strategyAdmissionPolicy(), nil) {
		t.Fatalf("predictive diagnostics must not veto admission: %s", decision.Reason)
	}

	if decision.Alternatives[predictiveReadyEvidenceKey] != 0 {
		t.Fatal("predictive state should remain visible as zero evidence")
	}
}

func TestLiquidityParticipatesInOrdinalCandidateRanking(t *testing.T) {
	thin := types.NewDecision(types.ActionEnter, "THIN/USD")
	deep := types.NewDecision(types.ActionEnter, "DEEP/USD")

	for _, decision := range []*types.Decision{thin, deep} {
		decision.Direction = 1
		decision.ThesisScore = 0.8
		decision.Confidence = 0.8
		decision.ThesisSupport = 1.2
		decision.ThesisContradiction = 0.1
		decision.GraphScore = 0.5
		alternativesOf(decision)[liquidityMassKey] = 1
	}

	alternativesOf(thin)[liquidityScoreKey] = -0.8
	alternativesOf(deep)[liquidityScoreKey] = 0.8
	rankAdmissionCandidates([]*types.Decision{thin, deep})

	if admissionOrder(deep, thin) >= 0 {
		t.Fatal("the otherwise-equal liquid candidate should rank first")
	}
}
