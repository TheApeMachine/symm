package resonance

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func measurementArtifact(measurement logic.Measurement) *datura.Artifact {
	categoryIndex := classifierIndex(measurement.Category)

	if categoryIndex <= 0 ||
		!logic.ScalarFinite(measurement.Confidence) ||
		measurement.Confidence <= 0 ||
		measurement.Symbol == "" {
		return nil
	}

	artifact := datura.Acquire("resonance", datura.Artifact_Type_json)

	if artifact == nil {
		return nil
	}

	artifact.WithRole("measurement")
	artifact.WithScope(measurement.Symbol)
	artifact.WithAttribute("classifier.category", categoryIndex)
	artifact.WithAttribute("classifier.confidence", measurement.Confidence)
	artifact.WithAttribute("classifier.strength", measurement.Strength)
	artifact.WithAttribute("price", measurement.Price)
	artifact.WithAttribute("volume", measurement.Volume)
	artifact.WithAttribute("spread", measurement.Spread)
	artifact.WithAttribute("elapsed", measurement.Elapsed)
	artifact.WithAttribute("surprise", measurement.Surprise)

	observedAt := measurement.ObservedAt

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	artifact.WithAttribute("observed_at", observedAt.UTC().Format(time.RFC3339Nano))

	payload, err := json.Marshal(measurement)

	if err != nil {
		artifact.Release()

		return nil
	}

	if artifact.WithPayload(payload) == nil {
		artifact.Release()

		return nil
	}

	return artifact
}

func classifierIndex(category logic.CategoryType) int {
	switch string(category) {
	case CategoryFlow:
		return 1
	case CategoryStress:
		return 2
	case CategoryCoupling:
		return 3
	default:
		return 0
	}
}
