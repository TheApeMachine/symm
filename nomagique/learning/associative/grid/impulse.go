package grid

import (
	"slices"
	"time"
)

/*
Impulse is the event-owned activation sequence inside the fixed regions.
Ready means formation has completed; a quiet sequence does not undo formation.

The impulse carries no trading semantics. It is the grid's current activation
pattern — a sequence of region tokens that the learner uses as context for
choosing an action. What the learner should do is the learner's decision.
*/
type Impulse struct {
	Label    string
	At, From time.Time
	Version  uint64
	Ready    bool
	Regions  []Region
}

/* Impulse copies the current activation sequence without rebuilding regions. */
func (grid *Space) Impulse(label string, at, from time.Time) (Impulse, error) {
	grid.mu.Lock()
	defer grid.mu.Unlock()

	contextLabel := label

	if contextLabel == "" {
		contextLabel = grid.UpdatedLabel
	}

	if contextLabel == "" {
		return Impulse{At: at, From: from, Ready: false}, nil
	}
	regions, version, err := grid.regionsLocked(contextLabel)

	if err != nil {
		return Impulse{}, err
	}

	return Impulse{
		Label: contextLabel, At: at, From: from, Version: version,
		Ready: grid.Formed, Regions: slices.Clone(regions),
	}, nil
}
