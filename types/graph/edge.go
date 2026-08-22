package graph

import "time"

/*
Edge represents a directed, weighted relationship from Node A to Node B.
*/
type Edge struct {
	From         string        `json:"from"`
	To           string        `json:"to"`
	Relation     RelationType  `json:"relation"`
	Weight       float64       `json:"weight"`
	Confidence   float64       `json:"confidence"`
	Quality      *float64      `json:"quality,omitempty"`
	Evidence     []string      `json:"evidence,omitempty"`
	ObservedFrom time.Time     `json:"observedFrom,omitempty"`
	Horizon      time.Duration `json:"horizon,omitempty"`
	At           time.Time     `json:"at"`
	Reason       string        `json:"reason,omitempty"`
}
