package resonance

import (
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/logic"
)

type settledSymbolEntry struct {
	outcome     settleOutcome
	measurement *datura.Artifact
	layers      []learning.ResonanceLayerWire
	surprise    float64
	energy      float64
}

/*
SettledSnapshot is the resonance batch row used for dashboard prediction frames.
*/
type SettledSnapshot struct {
	Scope    string
	Layers   []learning.ResonanceLayerWire
	Surprise float64
	Energy   float64
}

func buildSettledSymbolEntry(
	signal *Signal,
	outcome settleOutcome,
	measurement *datura.Artifact,
) (settledSymbolEntry, error) {
	if signal == nil {
		return settledSymbolEntry{}, fmt.Errorf("resonance: signal is nil")
	}

	var (
		layers   []learning.ResonanceLayerWire
		surprise float64
		energy   float64
	)

	if outcome.wireSource != nil {
		layers, surprise, energy = outcome.wireSource.WireSnapshot()
	}

	if outcome.wireSource == nil {
		layers = wireLayersFromBatch(signal.arch, outcome.input, outcome.latent, outcome.surprise)
		surprise = outcome.surprise
		energy = outcome.energy
	}

	if len(layers) == 0 {
		return settledSymbolEntry{}, fmt.Errorf("resonance: settled layers are empty")
	}

	return settledSymbolEntry{
		outcome:     outcome,
		measurement: measurement,
		layers:      layers,
		surprise:    surprise,
		energy:      energy,
	}, nil
}

const resonanceLatentWidth = 3

func requireLatentVector(latent []float64) error {
	if len(latent) != resonanceLatentWidth {
		return fmt.Errorf("resonance: latent vector width is %d, want %d", len(latent), resonanceLatentWidth)
	}

	for index, value := range latent {
		if err := requireFiniteResonance(fmt.Sprintf("latent[%d]", index), value); err != nil {
			return err
		}
	}

	return nil
}

func symbolSummaryRow(entry settledSymbolEntry) (map[string]any, error) {
	latent := append([]float64(nil), entry.outcome.latent...)

	if err := requireLatentVector(latent); err != nil {
		return nil, err
	}

	category := datura.Peek[string](entry.measurement, "category")

	if category == "" {
		categoryIndex := int(datura.Peek[float64](entry.measurement, "output", "value"))
		category = string(logic.Categories[categoryIndex])
	}

	scope, _ := entry.measurement.Scope()

	return map[string]any{
		"symbol":     scope,
		"surprise":   entry.surprise,
		"energy":     entry.energy,
		"confidence": datura.Peek[float64](entry.measurement, "output", "confidence"),
		"category":   category,
		"strength":   datura.Peek[float64](entry.measurement, "output", "strength"),
		"latent":     latent,
	}, nil
}

func focusSymbolIndex(settled []settledSymbolEntry) int {
	focusIndex := 0

	for index, entry := range settled {
		if entry.surprise > settled[focusIndex].surprise {
			focusIndex = index
		}
	}

	return focusIndex
}

func universeSnapshotPayload(
	arch []int,
	settled []settledSymbolEntry,
) (map[string]any, error) {
	if len(settled) == 0 {
		return nil, fmt.Errorf("resonance: universe snapshot has no settled symbols")
	}

	focusIndex := focusSymbolIndex(settled)
	focusEntry := settled[focusIndex]

	focusScope, _ := focusEntry.measurement.Scope()

	focusPayload, err := snapshotPayload(
		focusScope,
		arch,
		focusEntry.measurement,
		focusEntry.layers,
		focusEntry.surprise,
		focusEntry.energy,
	)

	if err != nil {
		return nil, err
	}

	symbols := make([]map[string]any, 0, len(settled))
	snapshots := make([]map[string]any, 0, len(settled))

	for _, entry := range settled {
		row, rowErr := symbolSummaryRow(entry)

		if rowErr != nil {
			return nil, rowErr
		}

		symbols = append(symbols, row)

		scope, _ := entry.measurement.Scope()
		snapshot, snapshotErr := snapshotPayload(
			scope,
			arch,
			entry.measurement,
			entry.layers,
			entry.surprise,
			entry.energy,
		)

		if snapshotErr != nil {
			return nil, snapshotErr
		}

		snapshots = append(snapshots, snapshot)
	}

	if focusEntry.measurement.Timestamp() <= 0 {
		return nil, fmt.Errorf("resonance: universe snapshot event time is zero")
	}

	observedAt := time.Unix(0, focusEntry.measurement.Timestamp()).UTC()

	return map[string]any{
		"type":         "resonance_universe",
		"ts":           observedAt.UTC().Format(time.RFC3339Nano),
		"arch":         arch,
		"symbol_count": len(settled),
		"focus_symbol": focusScope,
		"symbols":      symbols,
		"snapshots":    snapshots,
		"focus":        focusPayload,
	}, nil
}

func (signal *Signal) publishUniverse(settled []settledSymbolEntry) error {
	if signal == nil || signal.uiBroadcast == nil || len(settled) == 0 {
		return nil
	}

	payload, err := universeSnapshotPayload(signal.arch, settled)

	if err != nil {
		return err
	}

	artifact := datura.Acquire("resonance-universe", datura.Artifact_Type_json)
	artifact.WithRole("resonance")
	artifact.WithDestination("ui")

	marshaled, err := sonic.Marshal(payload)

	if err != nil {
		return fmt.Errorf("resonance: marshal universe snapshot: %w", err)
	}

	if artifact.WithPayload(marshaled) == nil {
		return fmt.Errorf("resonance: marshal universe snapshot: payload rejected")
	}

	return signal.uiBroadcast.Send(artifact)
}
