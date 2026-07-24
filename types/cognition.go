package types

import "time"

/*
CognitionBranch is one node in the sensory prefix tree exported for Cortex.
*/
type CognitionBranch struct {
	ID          int     `json:"id"`
	ParentID    int     `json:"parentId"`
	Token       string  `json:"token"`
	Prefix      string  `json:"prefix"`
	Depth       int     `json:"depth"`
	Probability float64 `json:"probability"`
	Count       uint64  `json:"count"`
}

/*
CognitionBeam is one scored beam-search path exported for Cortex.
*/
type CognitionBeam struct {
	Sequence string  `json:"sequence"`
	Score    float64 `json:"score"`
}

/*
CognitionClass is one attractor-basin posterior exported for Cortex.
*/
type CognitionClass struct {
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

/*
Cognition is the DMT reading for one symbol's current physical and signal
context. It keeps the persistent cognitive engine behind the Thesis boundary so
strategy can consume its readiness and ambiguity without depending on DMT.
*/
type Cognition struct {
	Source           string             `json:"source"`
	Symbol           string             `json:"symbol"`
	At               time.Time          `json:"at"`
	Sequence         string             `json:"sequence"`
	RegimePrefix     string             `json:"regimePrefix"`
	Winner           string             `json:"winner"`
	Ready            bool               `json:"ready"`
	Error            string             `json:"error,omitempty"`
	Confidence       float64            `json:"confidence"`
	Contrast         float64            `json:"contrast"`
	EntropyBits      float64            `json:"entropyBits"`
	EntropyThreshold float64            `json:"entropyThreshold"`
	Ambiguous        bool               `json:"ambiguous"`
	Cohort           uint64             `json:"cohort"`
	LookaheadScore   float64            `json:"lookaheadScore"`
	LookaheadPaths   int                `json:"lookaheadPaths"`
	BeamWidth        int                `json:"beamWidth"`
	MaxHops          int                `json:"maxHops"`
	NodeCount        int                `json:"nodeCount"`
	Predictions      map[string]float64 `json:"predictions"`
	Branches         []CognitionBranch  `json:"branches,omitempty"`
	Beams            []CognitionBeam    `json:"beams,omitempty"`
	Classes          []CognitionClass   `json:"classes,omitempty"`
	REMFrom          time.Time          `json:"remFrom,omitempty"`
	REMThrough       time.Time          `json:"remThrough,omitempty"`
	REMReplays       int                `json:"remReplays,omitempty"`
}
