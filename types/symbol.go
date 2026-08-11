package types

import (
	"slices"
	"sync"
)

/*
Symbol is an alternative grouping of measurements, and is used in logic that
legitimately needs to fan out measurements by symbol, rather than by source.
It should be kept extremely simple, and lean, and is not an invitation to
start adding additional complexity beyond what is truly earned. Symbols
carry their own readiness state, which is important for the resonance solver,
which needs a stable measurement set to train on.
*/
type Symbol struct {
	measurementMu       sync.RWMutex
	measurementRevision uint64
	Symbol              string         `json:"symbol,omitempty"`
	Status              Status         `json:"status,omitempty"`
	Measurements        []*Measurement `json:"measurements,omitempty"`
	Decisions           *sync.Map      `json:"decisions,omitempty"`
	Graphs              *sync.Map      `json:"graphs,omitempty"`
	Categories          *sync.Map      `json:"categories,omitempty"`
	Phase               *sync.Map      `json:"-"`
	Cognition           *sync.Map      `json:"-"`
	Resonance           *sync.Map      `json:"-"`
	Causal              *sync.Map      `json:"-"`
}

/*
NewSymbol creates empty measurement state for one market symbol.
*/
func NewSymbol(symbol string, ui chan []byte) *Symbol {
	return &Symbol{
		Symbol:       symbol,
		Status:       READY,
		Measurements: make([]*Measurement, 0),
		Decisions:    &sync.Map{},
		Graphs:       &sync.Map{},
		Categories:   &sync.Map{},
		Phase:        &sync.Map{},
		Cognition:    &sync.Map{},
		Resonance:    &sync.Map{},
		Causal:       &sync.Map{},
	}
}

func (symbol *Symbol) Reset() {
	symbol.measurementMu.Lock()
	defer symbol.measurementMu.Unlock()

	symbol.Status = READY
	symbol.Measurements = symbol.Measurements[:0]
	symbol.Decisions.Clear()
	symbol.Graphs.Clear()
	symbol.Categories.Clear()
	symbol.Phase.Clear()
	symbol.Cognition.Clear()
	symbol.Resonance.Clear()
	symbol.Causal.Clear()
}

/*
AddMeasurement retains the latest measurement for one source and peer and
reports whether that identity actually changed.
*/
func (symbol *Symbol) AddMeasurement(measurement *Measurement) bool {
	if symbol == nil || measurement == nil {
		return false
	}

	symbol.measurementMu.Lock()
	defer symbol.measurementMu.Unlock()

	for index, existing := range symbol.Measurements {
		if existing.Source == measurement.Source && existing.Peer == measurement.Peer {
			if existing.ID == measurement.ID {
				return false
			}

			symbol.Measurements[index] = measurement
			symbol.measurementRevision++
			return true
		}
	}

	symbol.Measurements = append(symbol.Measurements, measurement)
	symbol.measurementRevision++
	return true
}

/*
MeasurementsSnapshot returns an immutable view of the accumulated current evidence.
*/
func (symbol *Symbol) MeasurementsSnapshot() []*Measurement {
	if symbol == nil {
		return nil
	}

	symbol.measurementMu.RLock()
	defer symbol.measurementMu.RUnlock()

	return slices.Clone(symbol.Measurements)
}

/*
MeasurementState returns one immutable evidence cut and its monotonic revision.
*/
func (symbol *Symbol) MeasurementState() ([]*Measurement, uint64, bool) {
	symbol.measurementMu.RLock()
	defer symbol.measurementMu.RUnlock()

	return slices.Clone(symbol.Measurements), symbol.measurementRevision,
		len(symbol.Measurements) > 0
}
