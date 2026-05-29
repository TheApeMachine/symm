package market

import (
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/market/perspectives"
)

type PerspectiveType uint8

const (
	PerspectiveUnclear PerspectiveType = iota
	PerspectivePump
	PerspectiveDump
	PerspectiveMomentum
	PerspectiveBreakout
	PerspectivePullback
	PerspectiveReversal
	PerspectiveContinuation
	PerspectiveDivergence
	PerspectiveConvergence
)

/*
Perspective is one trade thesis encoded as a decision tree.

The new market layer works like this:

  - Signals continuously emit engine.Measurement values (each carries a
    Category string — the signal's row from DECISION.md).
  - Story holds the latest measurements and registers many Perspective values
    (pump, dump, organic trend, …). Each Perspective owns a *perspectives.Tree:
    a playbook of CategoryType branches ending in Action or Observation nodes.
  - To see if a thesis applies, walk the tree: at each CategoryType branch,
    check whether the current measurement set includes that category (via
    Measurement.HasCategory). If any required step is missing, that perspective
    is not traversable for this symbol and is ignored.
  - Observation nodes branch on ObservationType (holding, not holding, …) using
    runtime state; Category branches use ingested signal measurements only.
  - A traversable path that reaches an Action leaf is an active playbook
    (stop loss, take profit, etc.). Several perspectives can be active in parallel.

This type is the market-layer wrapper: PerspectiveType identifies which playbook,
Measurements is the ingested verdict set for one symbol, Tree is the static
structure from market/perspectives (e.g. NewPumpPerspective().Tree).
*/
type Perspective struct {
	Type         PerspectiveType
	Measurements []Measurement
	Regime       engine.MarketRegime
	Tree         *perspectives.Tree
}

/*
NewPerspective creates a perspective shell with room for measurements.
Tree is nil until a playbook is attached (see NewPumpPerspective in market).
*/
func NewPerspective(measurements []Measurement) *Perspective {
	capacity := len(measurements)

	if capacity == 0 {
		capacity = 16
	}

	return &Perspective{
		Type:         PerspectiveUnclear,
		Measurements: append([]Measurement(nil), measurements...),
		Regime:       engine.RegimeUnknown,
		Tree:         nil,
	}
}

/*
NewPumpPerspective returns the pump playbook with an empty measurement buffer.
*/
func NewPumpPerspective() *Perspective {
	playbook := perspectives.NewPumpPerspective()

	return &Perspective{
		Type:         PerspectivePump,
		Measurements: make([]Measurement, 0, 16),
		Regime:       engine.RegimeUnknown,
		Tree:         playbook.Tree,
	}
}

/*
Ingest adds or replaces the measurement for the same signal source.
*/
func (perspective *Perspective) Ingest(reading engine.Measurement) error {
	measurement, err := MeasurementFromEngine(reading)

	if err != nil {
		return err
	}

	for index, existing := range perspective.Measurements {
		if existing.Source == measurement.Source {
			perspective.Measurements[index] = measurement
			return nil
		}
	}

	perspective.Measurements = append(perspective.Measurements, measurement)

	return nil
}

/*
HasCategory reports whether any ingested measurement carries the branch key.
*/
func (perspective *Perspective) HasCategory(category perspectives.CategoryType) bool {
	for _, measurement := range perspective.Measurements {
		if measurement.HasCategory(category) {
			return true
		}
	}
	return false
}
