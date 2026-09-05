package learning

import (
	"math"
	"slices"

	"github.com/theapemachine/errnie"
)

/* Region is one uphill basin of simultaneous activity in the numeric plane. */
type Region struct {
	ID        uint64  `json:"id"`
	Strength  float64 `json:"strength"`
	Authority float64 `json:"authority"`
	Members   int     `json:"members"`
}

/*
regions owns reusable watershed storage. Raster resolution is ceil(sqrt(N)):
one cell per quantity on average in the requested two-dimensional plane.
Four-neighbor connectivity defines the raster topology. Equal-height connected
plateaus merge before uphill assignment; no selected cluster count or radius
participates. Otsu's between-class variance selects the active basins.
*/
type regions struct {
	heat, authority                    []float64
	count, peak, parent, uphill, index []int
	output                             []Region
}

/* Activity exposes borrowed quality-conditioned values for one context. */
func (grid *Grid) Activity(label string) ([]float64, []float64, error) {
	row, exists := grid.rowIndex[label]
	if !exists {
		return nil, nil, errnie.Err(errnie.NotFound, "grid: unknown context "+label, nil)
	}
	return grid.activations[row], grid.qualities[row], nil
}

/*
Regions returns the current hot basins, strongest first, in borrowed storage.
The version identifies this context's last update, not another context's activity.
IDs name the strongest contributing quantity at a basin's peak, not a human
label or a raster address that changes when the coordinates move.
*/
func (grid *Grid) Regions(label string) ([]Region, uint64, error) {
	row, exists := grid.rowIndex[label]

	if !exists {
		return nil, 0, errnie.Err(errnie.NotFound, "grid: unknown context "+label, nil)
	}

	for len(grid.regions) <= row {
		grid.regions = append(grid.regions, regions{})
	}

	return grid.regions[row].step(grid, row), grid.versions[row], nil
}

/* step rasterizes current evidenced movement, then follows its uphill basins. */
func (regions *regions) step(grid *Grid, row int) []Region {
	regions.output = regions.output[:0]
	width := int(math.Ceil(math.Sqrt(float64(len(grid.Columns)))))
	area := width * width

	if area == 0 {
		return regions.output
	}

	if len(regions.heat) != area {
		regions.heat, regions.authority = make([]float64, area), make([]float64, area)
		regions.count, regions.peak = make([]int, area), make([]int, area)
		regions.parent, regions.uphill, regions.index = make([]int, area), make([]int, area), make([]int, area)
	}

	clear(regions.heat)
	clear(regions.authority)
	clear(regions.count)
	minimum, maximum := *grid.Coordinates[0], *grid.Coordinates[0]

	for _, point := range grid.Coordinates {
		for dimension := range gridDimensions {
			minimum[dimension] = min(minimum[dimension], point[dimension])
			maximum[dimension] = max(maximum[dimension], point[dimension])
		}
	}

	for column, point := range grid.Coordinates {
		energy := grid.activations[row][column] * grid.activations[row][column]

		if energy == 0 || !grid.Present[row][column] {
			continue
		}

		cell := [gridDimensions]int{}

		for dimension := range gridDimensions {
			span := maximum[dimension] - minimum[dimension]

			if span > 0 {
				cell[dimension] = min(width-1, int((point[dimension]-minimum[dimension])/span*float64(width)))
			}
		}

		index := cell[1]*width + cell[0]
		previous := regions.peak[index]

		if regions.count[index] == 0 || energy > grid.activations[row][previous]*grid.activations[row][previous] {
			regions.peak[index] = column
		}

		regions.heat[index] += energy
		regions.authority[index] += energy * grid.qualities[row][column]
		regions.count[index]++
	}

	regions.watershed(width)
	regions.collect()
	return regions.output
}

/* root compresses a monotone basin path in place. */
func (regions *regions) root(cell int) int {
	root := cell

	for regions.parent[root] != root {
		root = regions.parent[root]
	}

	for cell != root {
		next := regions.parent[cell]
		regions.parent[cell] = root
		cell = next
	}

	return root
}

/* watershed joins plateaus, then attaches each one to its hottest neighbor. */
func (regions *regions) watershed(width int) {
	for cell := range regions.parent {
		regions.parent[cell], regions.uphill[cell], regions.index[cell] = cell, cell, -1
	}

	for cell, heat := range regions.heat {
		neighbors := [2]int{cell - 1, cell - width}

		if cell%width == 0 {
			neighbors[0] = -1
		}

		for _, neighbor := range neighbors {
			if neighbor < 0 || heat == 0 || regions.heat[neighbor] != heat {
				continue
			}

			left, right := regions.root(cell), regions.root(neighbor)
			regions.parent[max(left, right)] = min(left, right)
		}
	}

	for cell, heat := range regions.heat {
		neighbors := [4]int{cell - 1, cell + 1, cell - width, cell + width}

		if cell%width == 0 {
			neighbors[0] = -1
		}
		if cell%width == width-1 {
			neighbors[1] = -1
		}
		root := regions.root(cell)

		for _, neighbor := range neighbors {
			if neighbor < 0 || neighbor >= len(regions.heat) || heat == 0 {
				continue
			}

			if regions.heat[neighbor] > regions.heat[regions.uphill[root]] {
				regions.uphill[root] = neighbor
			}
		}
	}

	for cell := range regions.parent {
		if regions.parent[cell] == cell && regions.heat[regions.uphill[cell]] > regions.heat[cell] {
			regions.parent[cell] = regions.uphill[cell]
		}
	}
}

/* collect sums basins and retains the data-selected high-activity class. */
func (regions *regions) collect() {
	for cell, heat := range regions.heat {
		if heat == 0 {
			continue
		}

		root := regions.root(cell)
		index := regions.index[root]

		if index < 0 {
			index = len(regions.output)
			regions.index[root] = index
			regions.output = append(regions.output, Region{ID: uint64(regions.peak[root] + 1)})
		}

		region := &regions.output[index]
		region.Strength += heat
		region.Authority += regions.authority[cell]
		region.Members += regions.count[cell]
	}

	for index := range regions.output {
		region := &regions.output[index]
		region.Authority /= region.Strength
	}

	slices.SortFunc(regions.output, func(left, right Region) int {
		if left.Strength > right.Strength {
			return -1
		}
		if left.Strength < right.Strength {
			return 1
		}
		if left.ID < right.ID {
			return -1
		}
		if left.ID > right.ID {
			return 1
		}
		return 0
	})

	total, leading, best := 0.0, 0.0, 0.0
	keep := len(regions.output)

	for _, region := range regions.output {
		total += region.Strength
	}

	for index := 1; index < len(regions.output); index++ {
		leading += regions.output[index-1].Strength
		left, right := float64(index), float64(len(regions.output)-index)
		difference := leading/left - (total-leading)/right
		between := left * right * difference * difference

		if between > best {
			best, keep = between, index
		}
	}

	regions.output = regions.output[:keep]
}
