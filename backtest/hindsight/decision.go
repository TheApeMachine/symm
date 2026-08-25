package hindsight

import (
	"encoding/json"
	"fmt"
	"time"
)

/*
Decision is the window into the audit record that hindsight needs: what the
system decided, when, and — crucially — the live measurement scores it was
looking at (alternatives) and whether it had already classified the moment as
a genuine opportunity. Only these fields are decoded so this is the full
coupling between hindsight and the decision stream; the heavy types package is
not imported.
*/
type Decision struct {
	ID                      string             `json:"id"`
	Action                  string             `json:"action"`
	Symbol                  string             `json:"symbol"`
	At                      time.Time          `json:"at"`
	ThesisScore             float64            `json:"thesisScore"`
	ThesisConfidence        float64            `json:"thesisConfidence"`
	ThesisSupport           float64            `json:"thesisSupport"`
	ThesisContradiction     float64            `json:"thesisContradiction"`
	ThesisConditions        float64            `json:"thesisConditions"`
	Direction               float64            `json:"direction"`
	Confidence              float64            `json:"confidence"`
	AdmissionThreshold      float64            `json:"admissionGraphThreshold"`
	Opportunity             bool               `json:"opportunity"`
	OpportunityType         string             `json:"opportunityType,omitempty"`
	PredictiveReady         bool               `json:"predictiveReady"`
	PredictiveStatus        string             `json:"predictiveStatus"`
	Cause                   string             `json:"cause"`
	Reason                  string             `json:"reason"`
	Alternatives            map[string]float64 `json:"alternatives"`
	GraphScore              float64            `json:"graphScore"`
	AllocationClass         string             `json:"allocationClass"`
	AllocationHaircut       float64            `json:"allocation_haircut"`
	AllocationHaircutReason string             `json:"allocation_haircut_reason"`
	ReserveEligible         bool               `json:"reserveEligible"`
	ReserveReason           string             `json:"reserveReason"`
	OpenPositions           int                `json:"openPositions"`
	SlotCapacity            int                `json:"slotCapacity"`
}

/*
decisionsDecoder holds one raw audit event payload: a JSON array of decisions.
*/
func decodeDecisions(payload []byte) ([]Decision, error) {
	var decisions []Decision

	if err := json.Unmarshal(payload, &decisions); err != nil {
		return nil, fmt.Errorf("hindsight: decode decisions: %w", err)
	}

	return decisions, nil
}