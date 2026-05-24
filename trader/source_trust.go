package trader

import (
	"sync"

	"github.com/theapemachine/symm/engine"
)

const (
	sourceTrustFloor = 0.25
	sourceTrustCeil  = 2.0
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
	samples   int
	hitRate   float64
	magnitude float64
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
Apply ingests one settled forecast and updates hit-rate and magnitude EWMAs.
*/
func (store *SourceTrustStore) Apply(feedback engine.PredictionFeedback) {
	if store == nil || feedback.Unanchored || feedback.Source == "" ||
		feedback.PredictedReturn <= 0 {
		return
	}

	step, ok := engine.CalibrationStep(feedback.PredictedReturn, feedback.ActualReturn)

	if !ok {
		return
	}

	hit := 0.0

	if feedback.ActualReturn > 0 {
		hit = 1
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	entry := store.entryLocked(feedback.Source)
	alpha := sourceTrustAlpha(entry.samples)
	entry.samples++

	if entry.samples == 1 {
		entry.hitRate = hit
		entry.magnitude = step

		return
	}

	entry.hitRate += alpha * (hit - entry.hitRate)
	entry.magnitude += alpha * (step - entry.magnitude)
}

/*
Weight returns the live ensemble multiplier for one signal source.
*/
func (store *SourceTrustStore) Weight(source string) float64 {
	if store == nil || source == "" {
		return 1
	}

	store.mu.RLock()
	entry, ok := store.bySource[source]
	store.mu.RUnlock()

	if !ok || entry == nil || entry.samples == 0 {
		return 1
	}

	weight := entry.hitRate * entry.magnitude

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
