package grid

import (
	"math"
	"slices"
	"time"
)

// Impulse is the event-owned region output consumed after the grid barrier.
// Ready means a multi-quantity basin has more shared energy than unresolved energy.
type Impulse struct {
	Label    string
	At, From time.Time
	Version  uint64
	Ready    bool
	Regions  []Region
}

// Impulse copies only reduced regions; later grid turns cannot rewrite a
// decision already travelling through the downstream ring stage.
func (grid *Space) Impulse(label string, at, from time.Time) (Impulse, error) {
	regions, version, err := grid.Regions(label)

	if err != nil {
		return Impulse{}, err
	}
	row := grid.rowIndex[label]
	return Impulse{Label: label, At: at, From: from, Version: version,
		Ready: grid.reduced(row, regions), Regions: slices.Clone(regions)}, nil
}

// reduced admits an actual reduction only when its shared movement exceeds
// its residual plus measured uncertainty. This is an energy-resolution test,
// not a significance probability. Producer quality discounts unexplained
// movement; the sketch's spectral error bound charges every basin member.
// A hot singleton, immature input or unresolved lossy projection cannot qualify.
func (grid *Space) reduced(row int, basins []Region) bool {
	for _, basin := range basins {
		if basin.Members < 2 {
			continue
		}

		if grid.resolvedBasin(row, int(basin.ID)-1) {
			return true
		}
	}
	return false
}

// resolvedBasin fits one sign-aligned centroid to the basin profiles. Squared
// profile norms, residuals and the covariance error bound share the same units.
// Consistent inverse movement contributes to the same centroid.
func (grid *Space) resolvedBasin(row, peak int) bool {
	var centroid [gridDimensions]float64
	total, uncertainty, members := 0.0, 0.0, 0
	for column := range grid.Columns {
		if !grid.Present[row][column] || grid.regions[row].membership(column) != peak {
			continue
		}
		product := grid.basis[0][peak]*grid.basis[0][column] + grid.basis[1][peak]*grid.basis[1][column]
		sign := math.Copysign(1, product)
		for dimension := range gridDimensions {
			value := grid.basis[dimension][column]
			centroid[dimension] += sign * value
			total += value * value
			uncertainty += value * value * (1 - grid.qualities[row][column])
		}
		members++
	}

	if members < 2 || total == 0 {
		return false
	}
	shared := 0.0
	for _, sum := range centroid {
		shared += sum * sum / float64(members)
	}
	residual := total - shared
	uncertainty += float64(members) * grid.discarded
	return shared > residual+uncertainty
}

func (regions *regions) membership(column int) int {
	cell := regions.cells[column]

	if cell < 0 {
		return -1
	}
	return regions.peak[regions.root(cell)]
}
