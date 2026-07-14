package types

import "time"

/*
Hypothesis is one causal claim produced by logic from named evidence. It keeps
the tested treatment, controls, and future outcome explicit without carrying
logic implementation state on the Thesis.
*/
type Hypothesis struct {
	Source         SourceType `json:"source"`
	Symbol         string     `json:"symbol"`
	At             time.Time  `json:"at"`
	Samples        uint64     `json:"samples"`
	Ready          bool       `json:"ready"`
	Claim          string     `json:"claim"`
	Treatment      string     `json:"treatment"`
	Controls       []string   `json:"controls"`
	Outcome        string     `json:"outcome"`
	Association    float64    `json:"association"`
	Intervention   float64    `json:"intervention"`
	DoExpectation  float64    `json:"doExpectation"`
	Uplift         float64    `json:"uplift"`
	Counterfactual float64    `json:"counterfactual"`
	Confidence     float64    `json:"confidence"`
	Strength       float64    `json:"strength"`
}
