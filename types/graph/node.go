package graph

import "time"

/*
Node represents a discrete market entity, metric, category, or latent state.
*/
type Node struct {
	ID            string          `json:"id"`
	Symbol        string          `json:"symbol,omitempty"`
	Peer          string          `json:"peer,omitempty"`
	Source        string          `json:"source,omitempty"`
	MeasurementID string          `json:"measurementId,omitempty"`
	Metric        string          `json:"metric,omitempty"`
	Side          string          `json:"side,omitempty"`
	Kind          Kind            `json:"kind"`
	Value         float64         `json:"value"`
	Normalized    *float64        `json:"normalized,omitempty"`
	Quality       *float64        `json:"quality,omitempty"`
	Strength      float64         `json:"strength,omitempty"`
	Confidence    float64         `json:"confidence"`
	Maturity      float64         `json:"maturity,omitempty"`
	Unit          string          `json:"unit,omitempty"`
	ObservedFrom  time.Time       `json:"observedFrom,omitempty"`
	Horizon       time.Duration   `json:"horizon,omitempty"`
	At            time.Time       `json:"at"`
	Metadata      map[string]any  `json:"metadata,omitempty"`
}
