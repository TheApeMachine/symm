package grid

import (
	"slices"
	"time"
)

/*
Impulse is the event-owned activation sequence inside the fixed regions.
Ready means formation has completed; a quiet sequence does not undo formation.
*/
type Impulse struct {
	Label    string
	At, From time.Time
	Version  uint64
	Ready    bool
	Regions  []Region

	// Moment is what the record says this observation was, in the record's own
	// words. The grid neither reads it nor acts on it: a tape that knows where
	// its own ignition and exhaustion were is describing itself, and carrying
	// that through unchanged is what lets a learner be taught on it without the
	// grid acquiring an opinion about what any of it means.
	Moment string

	// Graded is how strongly the record stands behind that moment, and Grade
	// what it stands behind it with. A frame the record graded and one it did
	// not are different readings, so presence travels separately from value:
	// an ungraded observation strengthens a link on having been seen, while a
	// grade of zero is a judgement that it taught nothing.
	Grade  float64
	Graded bool
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
		Moment: grid.moment, Grade: grid.grade, Graded: grid.graded,
	}, nil
}
