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
}

/* Impulse copies the current activation sequence without rebuilding regions. */
func (grid *Space) Impulse(label string, at, from time.Time) (Impulse, error) {
	regions, version, err := grid.Regions(label)

	if err != nil {
		return Impulse{}, err
	}
	return Impulse{
		Label: label, At: at, From: from, Version: version,
		Ready: grid.Formed, Regions: slices.Clone(regions),
	}, nil
}
