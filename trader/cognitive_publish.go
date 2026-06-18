package trader

import (
	"github.com/theapemachine/symm/trader/cognitive"
	"github.com/theapemachine/symm/ui"
)

func cognitiveReadingWire(reading *cognitive.Reading) ui.CognitiveReadingWire {
	if reading == nil {
		return ui.CognitiveReadingWire{}
	}

	return ui.CognitiveReadingWire{
		Scope:            reading.Scope,
		Sequence:         string(reading.Sequence),
		RegimePrefix:     string(reading.RegimePrefix),
		RegimeCohort:     len(reading.RegimeCohort),
		Ambiguous:        reading.Ambiguous,
		Sideline:         reading.Sideline,
		EntropyBits:      reading.EntropyBits,
		EntropyThreshold: reading.EntropyThreshold,
		ClassConfidence:  reading.ClassConfidence,
		ContrastEvidence: reading.ContrastEvidence,
		LookaheadScore:   reading.LookaheadScore,
		LookaheadPaths:   len(reading.LookaheadPaths),
		WinnerClass:      string(reading.WinnerClass),
	}
}

func cognitiveReadingWires(readings []*cognitive.Reading) []ui.CognitiveReadingWire {
	wires := make([]ui.CognitiveReadingWire, 0, len(readings))

	for _, reading := range readings {
		if reading == nil || reading.Scope == "" {
			continue
		}

		wires = append(wires, cognitiveReadingWire(reading))
	}

	return wires
}

func (crypto *Crypto) publishCognitiveReadings(readings []*cognitive.Reading) error {
	if crypto == nil || crypto.pool == nil || len(readings) == 0 {
		return nil
	}

	return ui.PublishCognitiveReadings(crypto.pool, cognitiveReadingWires(readings))
}
