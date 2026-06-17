package logic

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
)

/*
MeasurementFromArtifact builds a logic.Measurement from classifier metadata
or from a JSON payload when the artifact already carries one.
*/
func MeasurementFromArtifact(
	signalName string,
	artifact *datura.Artifact,
) (Measurement, bool) {
	if artifact == nil {
		return Measurement{}, false
	}

	if measurement, ok := measurementFromJSONPayload(artifact); ok {
		return measurement, true
	}

	origin, _ := artifact.Origin()

	if signalName == "" {
		signalName = origin
	}

	scope, _ := artifact.Scope()
	categoryIndex := datura.Peek[int](artifact, "classifier.category")
	confidence := datura.Peek[float64](artifact, "classifier.confidence")

	if signalName == "" || scope == "" || categoryIndex <= 0 || confidence <= 0 {
		return Measurement{}, false
	}

	category := CategoryFromSignalName(signalName, categoryIndex)

	if category == "" {
		return Measurement{}, false
	}

	measurement := Measurement{
		Source:     SourceFromSignalOrigin(signalName),
		Symbol:     scope,
		Category:   category,
		Confidence: confidence,
		Strength:   datura.Peek[float64](artifact, "classifier.strength"),
		Price:      datura.Peek[float64](artifact, "price"),
		Volume:     datura.Peek[float64](artifact, "volume"),
		Spread:     datura.Peek[float64](artifact, "spread"),
		Surprise:   datura.Peek[float64](artifact, "surprise"),
		Elapsed:    datura.Peek[float64](artifact, "elapsed"),
		ObservedAt: observedAtFromArtifact(artifact),
	}

	return measurement, true
}

func observedAtFromArtifact(artifact *datura.Artifact) time.Time {
	raw := datura.Peek[string](artifact, "observed_at")

	if raw == "" {
		return time.Now()
	}

	parsed, parseErr := time.Parse(time.RFC3339Nano, raw)

	if parseErr != nil {
		parsed, parseErr = time.Parse(time.RFC3339, raw)
	}

	if parseErr != nil {
		return time.Now()
	}

	return parsed
}

func measurementFromJSONPayload(artifact *datura.Artifact) (Measurement, bool) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK || payload[0] != '{' {
		return Measurement{}, false
	}

	var measurement Measurement

	if sonic.Unmarshal(payload, &measurement) != nil || measurement.Source == "" {
		return Measurement{}, false
	}

	if measurement.Symbol == "" {
		measurement.Symbol, _ = artifact.Scope()
	}

	return measurement, true
}
