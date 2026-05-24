package trader

import (
	"sync"

	"github.com/theapemachine/symm/engine"
)

const (
	sourceTrustFloor     = 0.25
	sourceTrustCeil      = 2.0
	sourceTrustColdStart = 0.5
	maxTrustSample       = 2.0
)

/*
SourceTrustStore tracks per-signal forecast accuracy from settled predictions.
Weights feed the ensemble scorer in the decision engine.
*/
type SourceTrustStore struct {
	mu       sync.RWMutex
	bySource map[string]*sourceTrustEntry
}

type sourceTrustEntry struct {
	samples      int
	hitRate      float64
	winMagnitude float64
	lossSeverity float64
}

/*
NewSourceTrustStore creates an empty per-source trust ledger.
*/
func NewSourceTrustStore() *SourceTrustStore {
	return &SourceTrustStore{
		bySource: make(map[string]*sourceTrustEntry),
	}
}

/*
Apply ingests one settled forecast and updates hit-rate, win magnitude, and loss severity EWMAs.
*/
func (store *SourceTrustStore) Apply(feedback engine.PredictionFeedback) {
	if store == nil || feedback.Unanchored || feedback.Source == "" ||
		feedback.PredictedReturn <= 0 {
		return
	}

	hit := 0.0

	if feedback.ActualReturn > 0 {
		hit = 1
	}

	winSample := winMagnitudeSample(feedback.PredictedReturn, feedback.ActualReturn)
	lossSample := lossSeveritySample(feedback.PredictedReturn, feedback.ActualReturn)

	store.mu.Lock()
	defer store.mu.Unlock()

	entry := store.entryLocked(feedback.Source)
	alpha := sourceTrustAlpha(entry.samples)
	entry.samples++

	if entry.samples == 1 {
		entry.hitRate = hit
		entry.winMagnitude = winSample
		entry.lossSeverity = lossSample

		return
	}

	entry.hitRate += alpha * (hit - entry.hitRate)

	if winSample > 0 {
		if entry.winMagnitude <= 0 {
			entry.winMagnitude = winSample
		} else {
			entry.winMagnitude += alpha * (winSample - entry.winMagnitude)
		}
	}

	if lossSample > 0 {
		entry.lossSeverity += alpha * (lossSample - entry.lossSeverity)
	}
}

/*
Weight returns the live ensemble multiplier for one signal source.
Unknown sources start at sourceTrustColdStart until settled samples exist.
*/
func (store *SourceTrustStore) Weight(source string) float64 {
	if store == nil || source == "" {
		return sourceTrustColdStart
	}

	store.mu.RLock()
	entry, ok := store.bySource[source]
	store.mu.RUnlock()

	if !ok || entry == nil || entry.samples == 0 {
		return sourceTrustColdStart
	}

	winTerm := entry.hitRate

	if entry.winMagnitude > 0 {
		winTerm *= entry.winMagnitude
	}

	weight := winTerm / (1 + entry.lossSeverity)

	if weight <= 0 {
		return sourceTrustFloor
	}

	if weight < sourceTrustFloor {
		return sourceTrustFloor
	}

	if weight > sourceTrustCeil {
		return sourceTrustCeil
	}

	return weight
}

func winMagnitudeSample(predictedReturn, actualReturn float64) float64 {
	if actualReturn <= 0 || predictedReturn <= 0 {
		return 0
	}

	ratio := actualReturn / predictedReturn

	if ratio > maxTrustSample {
		return maxTrustSample
	}

	return ratio
}

func lossSeveritySample(predictedReturn, actualReturn float64) float64 {
	if actualReturn >= 0 || predictedReturn <= 0 {
		return 0
	}

	severity := -actualReturn / predictedReturn

	if severity > maxTrustSample {
		return maxTrustSample
	}

	return severity
}

func (store *SourceTrustStore) entryLocked(source string) *sourceTrustEntry {
	entry, ok := store.bySource[source]

	if ok {
		return entry
	}

	entry = &sourceTrustEntry{}
	store.bySource[source] = entry

	return entry
}

func sourceTrustAlpha(samples int) float64 {
	if samples < 3 {
		return 1 / float64(samples+1)
	}

	return 0.15
}
