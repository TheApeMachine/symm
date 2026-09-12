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

/* impulse copies the current activation sequence without rebuilding regions. */
func (op *Space) impulse(label string, at, from time.Time) (Impulse, error) {
	op.mu.Lock()
	defer op.mu.Unlock()

	contextLabel := label

	if contextLabel == "" {
		contextLabel = op.updated
	}

	if contextLabel == "" {
		return Impulse{At: at, From: from, Ready: false}, nil
	}
	measured, version, err := op.regionsLocked(contextLabel)

	if err != nil {
		return Impulse{}, err
	}

	return Impulse{
		Label: contextLabel, At: at, From: from, Version: version,
		Ready: op.formed, Regions: slices.Clone(measured),
	}, nil
}
