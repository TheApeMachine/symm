package ui

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

/*
GaugeReadingFromArtifact maps one measurement artifact to dashboard gauge wire.
*/
func GaugeReadingFromArtifact(artifact *datura.Artifact) map[string]any {
	if artifact == nil {
		return nil
	}

	source := string(logic.ArtifactOrigin(artifact))

	if source == "" {
		return nil
	}

	categoryIndex := datura.Peek[int](artifact, "classifier", "category")
	category := string(logic.Categories[categoryIndex])

	if category == "" {
		category = datura.Peek[string](artifact, "category")
	}

	scope, _ := artifact.Scope()
	readingsCapacity := viper.GetInt("signals.feed_ring_capacity")

	if readingsCapacity <= 0 {
		readingsCapacity = 1024
	}

	return map[string]any{
		"type":               "gauge",
		"source":             source,
		"symbol":             scope,
		"confidence":         logic.ArtifactConfidence(artifact),
		"strength":           logic.ArtifactStrength(artifact),
		"surprise":           logic.ArtifactSurprise(artifact),
		"elapsed":            logic.ArtifactElapsed(artifact),
		"category":           category,
		"observed_at":        datura.Peek[string](artifact, "observed_at"),
		"calibrated":         true,
		"readings_capacity":  readingsCapacity,
		"surprise_threshold": 2.0,
	}
}

/*
StateFrame builds the dashboard state websocket payload from measurement artifacts.
*/
func StateFrame(
	measurements []*datura.Artifact,
	storyTicks uint64,
	walk *logic.WalkTrace,
) map[string]any {
	gaugeReadings := gaugeReadingsFromMeasurements(measurements)

	frame := map[string]any{
		"type":           "state",
		"story_ticks":    storyTicks,
		"gauge_readings": gaugeReadings,
		"measurements":   gaugeReadings,
	}

	if walk != nil {
		frame["walk"] = walk
		frame["playbook_evaluations"] = len(walk.Steps)
	}

	return frame
}

func gaugeReadingsFromMeasurements(measurements []*datura.Artifact) []map[string]any {
	readings := make([]map[string]any, 0, len(measurements))

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		reading := GaugeReadingFromArtifact(measurement)

		if reading == nil {
			continue
		}

		readings = append(readings, reading)
	}

	return readings
}
