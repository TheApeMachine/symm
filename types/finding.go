package types

/*
Finding is one evidence-backed PostMortem result attributed to a named system
layer. It records an observed effect and the validation required before any
candidate adjustment may change a running model.
*/
type Finding struct {
	Symbol             string   `json:"symbol"`
	Component          string   `json:"component"`
	Condition          string   `json:"condition"`
	Evidence           []string `json:"evidence"`
	EstimatedEffect    float64  `json:"estimatedEffect"`
	Uncertainty        float64  `json:"uncertainty"`
	ProposedAdjustment string   `json:"proposedAdjustment,omitempty"`
	RequiredValidation string   `json:"requiredValidation"`
	CurrentModel       string   `json:"currentModel,omitempty"`
	CandidateModel     string   `json:"candidateModel,omitempty"`
}
