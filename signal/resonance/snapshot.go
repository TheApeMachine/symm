package resonance

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/logic"
)

func layerWireRows(layers []learning.ResonanceLayerWire) []map[string]any {
	rows := make([]map[string]any, 0, len(layers))

	for layerIndex, layer := range layers {
		rows = append(rows, map[string]any{
			"index":      layerIndex,
			"state":      layer.State,
			"prediction": layer.Prediction,
			"error_norm": layer.ErrorNorm,
		})
	}

	return rows
}

func requireFiniteResonance(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("resonance: %s is non-finite", name)
	}

	return nil
}

func snapshotPayload(
	scope string,
	arch []int,
	measurement logic.Measurement,
	layers []learning.ResonanceLayerWire,
	surprise float64,
	energy float64,
) (map[string]any, error) {
	if scope == "" {
		return nil, fmt.Errorf("resonance: snapshot scope is empty")
	}

	if measurement.ObservedAt.IsZero() {
		return nil, fmt.Errorf("resonance: snapshot event time is zero")
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("resonance: snapshot layers are empty")
	}

	if err := requireFiniteResonance("surprise", surprise); err != nil {
		return nil, err
	}

	if err := requireFiniteResonance("energy", energy); err != nil {
		return nil, err
	}

	if err := requireFiniteResonance("confidence", measurement.Confidence); err != nil {
		return nil, err
	}

	return map[string]any{
		"type":       "resonance",
		"symbol":     scope,
		"ts":         measurement.ObservedAt.UTC().Format(time.RFC3339Nano),
		"arch":       arch,
		"surprise":   surprise,
		"energy":     energy,
		"confidence": measurement.Confidence,
		"strength":   measurement.Strength,
		"category":   string(measurement.Category),
		"layers":     layerWireRows(layers),
	}, nil
}
