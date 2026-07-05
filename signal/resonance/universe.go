package resonance

import (
	"fmt"
	"time"

	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/logic"
)

type settledSymbolEntry struct {
	outcome     settleOutcome
	measurement *logic.Measurement
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
	measurement *logic.Measurement,
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

	category := string(entry.measurement.DominantCategory())

	if category == "" || category == string(logic.CategoryTypeNone) {
		category = entry.measurement.Status
	}

	return map[string]any{
		"symbol":     entry.measurement.Symbol,
		"surprise":   entry.surprise,
		"energy":     entry.energy,
		"confidence": entry.measurement.Confidence,
		"category":   category,
		"strength":   entry.measurement.Strength,
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

	focusPayload, err := snapshotPayload(
		focusEntry.measurement.Symbol,
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

		snapshot, snapshotErr := snapshotPayload(
			entry.measurement.Symbol,
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

	if focusEntry.measurement.At.IsZero() {
		return nil, fmt.Errorf("resonance: universe snapshot event time is zero")
	}

	observedAt := focusEntry.measurement.At.UTC()

	return map[string]any{
		"type":         "resonance_universe",
		"ts":           observedAt.UTC().Format(time.RFC3339Nano),
		"arch":         arch,
		"symbol_count": len(settled),
		"focus_symbol": focusEntry.measurement.Symbol,
		"symbols":      symbols,
		"snapshots":    snapshots,
		"focus":        focusPayload,
	}, nil
}

func (signal *Signal) publishUniverse(settled []settledSymbolEntry) error {
	if signal == nil || len(settled) == 0 {
		return nil
	}

	payload, err := universeSnapshotPayload(signal.arch, settled)
	if err != nil {
		return err
	}

	signal.snapshot = payload

	return nil
}

func (signal *Signal) DashboardSnapshot() (logic.SourceType, map[string]any, error) {
	payload := signal.snapshot
	signal.snapshot = nil

	return logic.SourceResonance, payload, nil
}
