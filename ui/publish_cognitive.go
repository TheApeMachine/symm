package ui

import (
	"github.com/theapemachine/qpool"
)

/*
CognitiveReadingWire is one sealed scope reading for dashboard telemetry.
*/
type CognitiveReadingWire struct {
	Scope            string
	Sequence         string
	RegimePrefix     string
	RegimeCohort     int
	Ambiguous        bool
	Sideline         bool
	EntropyBits      float64
	EntropyThreshold float64
	ClassConfidence  float64
	ContrastEvidence float64
	LookaheadScore   float64
	LookaheadPaths   int
	WinnerClass      string
}

/*
CognitiveFrame builds the dashboard wire shape for one cognitive reading.
*/
func CognitiveFrame(reading CognitiveReadingWire) map[string]any {
	if reading.Scope == "" {
		return nil
	}

	return map[string]any{
		"type":              "cognitive",
		"scope":             reading.Scope,
		"sequence":          reading.Sequence,
		"regime_prefix":     reading.RegimePrefix,
		"regime_cohort":     reading.RegimeCohort,
		"ambiguous":         reading.Ambiguous,
		"sideline":          reading.Sideline,
		"entropy_bits":      reading.EntropyBits,
		"entropy_threshold": reading.EntropyThreshold,
		"class_confidence":  reading.ClassConfidence,
		"contrast_evidence": reading.ContrastEvidence,
		"lookahead_score":   reading.LookaheadScore,
		"lookahead_paths":   reading.LookaheadPaths,
		"winner_class":      reading.WinnerClass,
	}
}

/*
PublishCognitive ships one cognitive reading frame to ui subscribers.
*/
func PublishCognitive(
	pool *qpool.Q[any],
	reading CognitiveReadingWire,
) error {
	frame := CognitiveFrame(reading)

	if frame == nil {
		return nil
	}

	return PublishPayload(pool, "cognitive", frame)
}

/*
PublishCognitiveReadings ships every sealed reading from the measure tick.
*/
func PublishCognitiveReadings(
	pool *qpool.Q[any],
	readings []CognitiveReadingWire,
) error {
	for _, reading := range readings {
		if err := PublishCognitive(pool, reading); err != nil {
			return err
		}
	}

	return nil
}
