package types

import (
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
	Readiness
	Status       Status         `json:"status,omitempty"`
	Measurements []*Measurement `json:"measurements,omitempty"`
	Decisions    *sync.Map      `json:"decisions,omitempty"`
	Graphs       *sync.Map      `json:"graphs,omitempty"`
	Categories   *sync.Map      `json:"categories,omitempty"`
	Phase        *sync.Map      `json:"-"`
	Cognition    *sync.Map      `json:"-"`
	Resonance    *sync.Map      `json:"-"`
	Causal       *sync.Map      `json:"-"`
}

/*
NewSymbol creates empty measurement state for one market symbol.
*/
func NewSymbol(symbol string, ui chan []byte) *Symbol {
	return &Symbol{
		Status:       READY,
		Readiness:    NewReadiness(symbol, ui),
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
	symbol.Readiness.Reset()
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
AddMeasurement retains the latest measurement for one source and peer.
*/
func (symbol *Symbol) AddMeasurement(measurement *Measurement) {
	symbol.Readiness.ResetLogic(measurement.Source)

	symbol.Measurements = append(symbol.Measurements, measurement)
	symbol.Stamp(measurement.Source)

	if symbol.SignalsMeasured() {
		symbol.Status = BUSY
	}
}

func (symbol *Symbol) Stamp(source SourceType) {
	symbol.Readiness.Stamp(source)

	if symbol.LogicAnalyzed() {
		symbol.Measurements = symbol.Measurements[:0]
		symbol.ResetSignals()
		symbol.Status = READY
	}
}

func (symbol *Symbol) Stamped(source SourceType) bool {
	return symbol.Readiness.Stamped(source)
}
