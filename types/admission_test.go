package types

import (
	"strings"
	"testing"
)

func requestedAdmissionPolicy() AdmissionPolicy {
	return AdmissionPolicy{
		RequiredDirection:    1,
		MinimumThesisScore:   0.7,
		MinimumConfidence:    0.7,
		MinimumSupport:       1,
		MaximumContradiction: 0.3,
	}
}

func TestAdmissionPolicyAcceptsRequestedBoundary(t *testing.T) {
	decision := Decision{
		Direction:           1,
		ThesisScore:         0.7,
		Confidence:          0.7,
		ThesisSupport:       1,
		ThesisContradiction: 0.3,
		PredictiveReady:     false,
	}
	result := requestedAdmissionPolicy().Evaluate(decision)

	if !result.Accepted {
		t.Fatalf("boundary decision should be admitted: %s", result.Explanation())
	}
}

func TestAdmissionPolicyReportsEveryFailure(t *testing.T) {
	decision := Decision{
		Direction:           -1,
		ThesisScore:         0.69,
		Confidence:          0.69,
		ThesisSupport:       0.99,
		ThesisContradiction: 0.31,
	}
	result := requestedAdmissionPolicy().Evaluate(decision)

	if result.Accepted {
		t.Fatal("decision should be rejected")
	}

	if len(result.Failures) != 5 {
		t.Fatalf("expected five independent failures, got %d", len(result.Failures))
	}

	for _, name := range []string{
		"direction", "thesis score", "confidence", "support", "contradiction",
	} {
		if !strings.Contains(result.Explanation(), name) {
			t.Fatalf("explanation %q is missing %q", result.Explanation(), name)
		}
	}
}
