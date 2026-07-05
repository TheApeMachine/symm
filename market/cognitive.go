package market

import "github.com/theapemachine/symm/logic"

/*
CognitiveReading is the per-symbol cognitive summary the Cortex surface renders.
The DMT evaluator builds it, but the UI and trader consume it as plain state.
*/
type CognitiveReading struct {
	Scope            string            `json:"scope"`
	Sequence         string            `json:"sequence"`
	RegimePrefix     string            `json:"regimePrefix"`
	RegimeCohort     int               `json:"regimeCohort"`
	Ambiguous        bool              `json:"ambiguous"`
	Sideline         bool              `json:"sideline"`
	EntropyBits      float64           `json:"entropyBits"`
	EntropyThreshold float64           `json:"entropyThreshold"`
	Surprisal        float64           `json:"surprisal"`
	Surprise         float64           `json:"surprise"`
	ClassConfidence  float64           `json:"classConfidence"`
	ContrastEvidence float64           `json:"contrastEvidence"`
	LookaheadScore   float64           `json:"lookaheadScore"`
	LookaheadPaths   int               `json:"lookaheadPaths"`
	WinnerClass      string            `json:"winnerClass"`
	UpdatedAt        int64             `json:"updatedAt"`
	BeamWidth        int               `json:"beamWidth"`
	MaxHops          int               `json:"maxHops"`
	NodeCount        int               `json:"nodeCount"`
	Branches         []CognitiveBranch `json:"branches,omitempty"`
	Beams            []CognitiveBeam   `json:"beams,omitempty"`
	Classes          []CognitiveClass  `json:"classes,omitempty"`
}

type CognitiveBranch struct {
	ID          int     `json:"id"`
	ParentID    int     `json:"parentId"`
	Token       string  `json:"token"`
	Prefix      string  `json:"prefix"`
	Depth       int     `json:"depth"`
	Probability float64 `json:"probability"`
	Count       uint64  `json:"count"`
}

type CognitiveBeam struct {
	Sequence string  `json:"sequence"`
	Score    float64 `json:"score"`
}

type CognitiveClass struct {
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

/*
ApplyCognitiveReadings stamps live measurements with the cognitive state read
from the backend evaluator.
*/
func ApplyCognitiveReadings(
	measurements []*logic.Measurement,
	readings map[string]CognitiveReading,
) {
	if len(measurements) == 0 || len(readings) == 0 {
		return
	}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.Symbol == "" {
			continue
		}

		reading, ok := readings[measurement.Symbol]

		if !ok {
			continue
		}

		surprise := reading.Surprise

		if surprise == 0 && reading.EntropyThreshold > 0 {
			surprise = reading.EntropyBits / reading.EntropyThreshold
		}

		if measurement.Metrics == nil {
			measurement.Metrics = map[string]float64{}
		}

		measurement.Metrics["surprisal"] = reading.Surprisal
		measurement.Surprise = surprise
		measurement.Metrics["surpriseThreshold"] = reading.EntropyThreshold
		measurement.Metrics["cognitiveClassConfidence"] = reading.ClassConfidence
		measurement.Status = cognitiveMeasurementStatus(measurement, reading)
	}
}

func cognitiveMeasurementStatus(
	measurement *logic.Measurement,
	reading CognitiveReading,
) string {
	confidence := measurement.Confidence
	strength := measurement.Strength

	if confidence <= 0 || strength <= 0 {
		return "standby"
	}

	if reading.Sideline || reading.Ambiguous {
		return "ambiguous"
	}

	if reading.ClassConfidence <= 0 {
		return "calibrating"
	}

	if measurement.Source == logic.SourceCausal && !measurement.CounterfactualReady {
		return "calibrating"
	}

	return "measured"
}
