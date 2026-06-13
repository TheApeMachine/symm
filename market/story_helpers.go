package market

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/logic"
)

func (story *Story) ensureActionIDs(action *logic.Action) {
	if action == nil {
		return
	}

	if action.DecisionID == "" {
		action.DecisionID = uuid.New().String()
	}

	if action.ActionID == "" {
		action.ActionID = uuid.New().String()
	}
}

func (story *Story) gaugeWireReadings(
	measurements []logic.Measurement,
) []GaugeReading {
	readings := make([]GaugeReading, 0, len(measurements))
	readingsCapacity := len(measurements)

	for _, measurement := range measurements {
		readings = append(readings, GaugeReading{
			Chart:            "gauge",
			Source:           measurement.Source,
			Symbol:           measurement.Symbol,
			Confidence:       measurement.Confidence,
			Surprise:         measurement.Surprise,
			Strength:         measurement.Strength,
			Elapsed:          measurement.Elapsed,
			Category:         measurement.Category,
			ObservedAt:       measurement.ObservedAt,
			ActiveReadings:   1,
			ReadingsCapacity: readingsCapacity,
			Calibrated:       true,
			BestEffort:       measurement.BestEffort,
			GapReason:        measurement.GapReason,
		})
	}

	return readings
}

func (story *Story) decisionEvidenceTTL() time.Duration {
	maxAge := story.tradingConfig.MaxQuoteAge
	transitTTL := story.tradingConfig.EntryTransitTTL

	if maxAge <= 0 {
		return transitTTL
	}

	if transitTTL <= 0 {
		return maxAge
	}

	if transitTTL < maxAge {
		return transitTTL
	}

	return maxAge
}

func (story *Story) recordNoActionTrace(walkTrace *logic.WalkTrace) error {
	if story.recorder == nil {
		return nil
	}

	return story.recorder.Write(map[string]any{
		"channel": "diagnostic",
		"type":    "playbook_no_action",
		"trace":   walkTrace.EvaluationSummary(),
	})
}
