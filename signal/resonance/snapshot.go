package resonance

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/types"
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
	measurement *types.Measurement,
	layers []learning.ResonanceLayerWire,
	surprise float64,
	energy float64,
) (map[string]any, error) {
	if scope == "" {
		return nil, fmt.Errorf("resonance: snapshot scope is empty")
	}

	if measurement == nil {
		return nil, fmt.Errorf("resonance: snapshot measurement is nil")
	}

	if measurement.At.IsZero() {
		return nil, fmt.Errorf("resonance: snapshot event time is zero")
	}

	observedAt := measurement.At.UTC()

	if len(layers) == 0 {
		return nil, fmt.Errorf("resonance: snapshot layers are empty")
	}

	if err := requireFiniteResonance("surprise", surprise); err != nil {
		return nil, err
	}

	if err := requireFiniteResonance("energy", energy); err != nil {
		return nil, err
	}

	confidence := 0.0
	strength := 0.0
	category := measurement.Status

	for _, categoryRow := range measurement.Categories {
		if categoryRow.Confidence <= confidence {
			continue
		}

		confidence = categoryRow.Confidence
		strength = categoryRow.Strength
		category = string(categoryRow.Type)
	}

	if err := requireFiniteResonance("confidence", confidence); err != nil {
		return nil, err
	}

	if category == "" || category == string(types.CategoryTypeNone) {
		category = measurement.Status
	}

	return map[string]any{
		"type":       "resonance",
		"symbol":     scope,
		"ts":         observedAt.UTC().Format(time.RFC3339Nano),
		"arch":       arch,
		"surprise":   surprise,
		"energy":     energy,
		"confidence": confidence,
		"strength":   strength,
		"category":   category,
		"layers":     layerWireRows(layers),
	}, nil
}
