package types

import (
	"fmt"
	"math"
	"strings"
	"time"
)


/*
AdmissionPolicy is the explicit temporary entry contract requested by the
operator. It lives in one typed value so strategy and the broker's final
execution boundary cannot silently drift apart.
*/
type AdmissionPolicy struct {
	RequiredDirection    float64 `json:"requiredDirection"`
	MinimumThesisScore   float64 `json:"minimumThesisScore"`
	MinimumConfidence    float64 `json:"minimumConfidence"`
	MinimumSupport       float64 `json:"minimumSupport"`
	MaximumContradiction float64 `json:"maximumContradiction"`
}

/*
AdmissionFailure is one independently failed policy dimension. Observed and
Boundary remain machine-readable while Message is suitable for the decision
journal and hindsight diagnosis.
*/
type AdmissionFailure struct {
	Name     string  `json:"name"`
	Observed float64 `json:"observed"`
	Boundary float64 `json:"boundary"`
	Message  string  `json:"message"`
}

/* AdmissionResult reports the complete policy result rather than one gate. */
type AdmissionResult struct {
	Accepted bool               `json:"accepted"`
	Failures []AdmissionFailure `json:"failures,omitempty"`
}

func finiteAdmissionValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
Evaluate checks every dimension and returns every failure. A caller therefore
never learns merely that a boolean gate failed; it receives the exact values
that prevented entry.
*/
func (policy AdmissionPolicy) Evaluate(decision Decision) AdmissionResult {
	failures := make([]AdmissionFailure, 0, 5)

	if !finiteAdmissionValue(decision.Direction) ||
		decision.Direction != policy.RequiredDirection {
		failures = append(failures, AdmissionFailure{
			Name: "direction", Observed: decision.Direction,
			Boundary: policy.RequiredDirection,
			Message: fmt.Sprintf(
				"direction %.4f must equal %.4f",
				decision.Direction, policy.RequiredDirection,
			),
		})
	}

	if !finiteAdmissionValue(decision.ThesisScore) ||
		decision.ThesisScore < policy.MinimumThesisScore {
		failures = append(failures, AdmissionFailure{
			Name: "thesis_score", Observed: decision.ThesisScore,
			Boundary: policy.MinimumThesisScore,
			Message: fmt.Sprintf(
				"thesis score %.4f is below %.4f",
				decision.ThesisScore, policy.MinimumThesisScore,
			),
		})
	}

	if !finiteAdmissionValue(decision.Confidence) ||
		decision.Confidence < policy.MinimumConfidence {
		failures = append(failures, AdmissionFailure{
			Name: "confidence", Observed: decision.Confidence,
			Boundary: policy.MinimumConfidence,
			Message: fmt.Sprintf(
				"confidence %.4f is below minimum confidence floor %.4f",
				decision.Confidence, policy.MinimumConfidence,
			),
		})
	}

	if !finiteAdmissionValue(decision.ThesisSupport) ||
		decision.ThesisSupport < policy.MinimumSupport {
		failures = append(failures, AdmissionFailure{
			Name: "support", Observed: decision.ThesisSupport,
			Boundary: policy.MinimumSupport,
			Message: fmt.Sprintf(
				"support %.4f is below %.4f",
				decision.ThesisSupport, policy.MinimumSupport,
			),
		})
	}

	if !finiteAdmissionValue(decision.ThesisContradiction) ||
		decision.ThesisContradiction > policy.MaximumContradiction {
		failures = append(failures, AdmissionFailure{
			Name: "contradiction", Observed: decision.ThesisContradiction,
			Boundary: policy.MaximumContradiction,
			Message: fmt.Sprintf(
				"contradiction %.4f exceeds %.4f",
				decision.ThesisContradiction, policy.MaximumContradiction,
			),
		})
	}

	return AdmissionResult{Accepted: len(failures) == 0, Failures: failures}
}

/* Explanation joins all failed dimensions for an operator-facing reason. */
func (result AdmissionResult) Explanation() string {
	parts := make([]string, 0, len(result.Failures))

	for _, failure := range result.Failures {
		parts = append(parts, failure.Message)
	}

	return strings.Join(parts, "; ")
}

/*
HindsightFeedback provides delayed opportunity-level attribution to the regulator.
It records the resolved outcome of trading opportunities and matured decisions,
capturing policy attribution without leaking future information into live decisions.
*/
type HindsightFeedback struct {
	At                  time.Time     `json:"at"`
	Symbol              string        `json:"symbol"`
	Opportunity         bool          `json:"opportunity"`
	OpportunityType     string        `json:"opportunityType,omitempty"`
	Captured            bool          `json:"captured"`
	Missed              bool          `json:"missed"`
	RealizedReturn      float64       `json:"realizedReturn"`
	MissedReturn        float64       `json:"missedReturn"`
	HoldingDuration     time.Duration `json:"holdingDuration"`
	DominantBlocker     string        `json:"dominantBlocker,omitempty"`
	ThesisMargin        float64       `json:"thesisMargin"`
	ConfidenceMargin    float64       `json:"confidenceMargin"`
	SupportMargin       float64       `json:"supportMargin"`
	ContradictionMargin float64       `json:"contradictionMargin"`
	GraphMargin         float64       `json:"graphMargin"`
}

