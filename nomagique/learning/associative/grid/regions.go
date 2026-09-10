package grid

import (
	"math"
	"slices"

	"github.com/theapemachine/errnie"
)

/* Region projects current activation inside one fixed sympathetic community. */
type Region struct {
	Condition uint64  `json:"condition"`
	Level     float64 `json:"level"`
	Change    float64 `json:"change"`
	ID        uint64  `json:"id"`
	Strength  float64 `json:"strength"`
	Authority float64 `json:"authority"`
	Members   int     `json:"members"`
}

/*
regions owns a fixed partition of the calibrated signed affinity graph.
Membership is structural. Current activation and its Otsu selection are a
projection and cannot create, delete or rename these communities.
*/
type regions struct {
	membership []int
	anchors    []int
	output     []Region
}

/*
form improves signed agreement until no single quantity can improve its
assignment. Stable direct and inverse relationships attract; inconsistent
relationships penalize sharing a community. Only strictly improving moves are
accepted, so this finite partition problem needs no chosen iteration count or
market threshold. It finds a local optimum, not a claimed global optimum.
*/
func (regions *regions) form(grid *Space) {
	regions.membership = make([]int, len(grid.Columns))

	for column := range regions.membership {
		regions.membership[column] = column
	}
	scores := make([]float64, len(grid.Columns))

	for {
		moved := false

		for column := range regions.membership {
			clear(scores)

			for peer, reading := range grid.graph[column] {
				weight := reading.strength() * (grid.weights[column] * grid.weights[peer])

				if !reading.stable() {
					weight = -weight
				}
				scores[regions.membership[peer]] += weight
			}
			current := regions.membership[column]
			best := current

			for candidate, score := range scores {
				if score > scores[best] {
					best = candidate
				}
			}

			if best != current {
				regions.membership[column] = best
				moved = true
			}
		}

		if !moved {
			break
		}
	}
	regions.anchors = regions.anchors[:0]
	indices := make(map[int]int)

	for column, community := range regions.membership {
		index, found := indices[community]

		if !found {
			index = len(regions.anchors)
			indices[community] = index
			regions.anchors = append(regions.anchors, column)
		}
		regions.membership[column] = index
	}
}

/* Activity exposes borrowed quality-conditioned values for one context. */
func (grid *Space) Activity(label string) ([]float64, []float64, error) {
	row, exists := grid.rowIndex[label]

	if !exists {
		return nil, nil, errnie.Error(errnie.Err(errnie.NotFound, "grid: unknown context "+label, nil))
	}
	return grid.activations[row], grid.qualities[row], nil
}

/*
Regions measures activation within the completed partition. Region identities
and membership survive missing or quiet readings. The returned active sequence
is borrowed storage; Impulse makes the immutable event-owned copy.
*/
func (grid *Space) Regions(label string) ([]Region, uint64, error) {
	row, exists := grid.rowIndex[label]

	if !exists {
		return nil, 0, errnie.Error(errnie.Err(errnie.NotFound, "grid: unknown context "+label, nil))
	}

	if !grid.Formed {
		return nil, grid.versions[row], nil
	}
	regions := &grid.regions
	regions.output = slices.Grow(regions.output[:0], len(regions.anchors))[:len(regions.anchors)]

	for index, anchor := range regions.anchors {
		regions.output[index] = Region{ID: uint64(anchor + 1)}
	}

	for column, community := range regions.membership {
		region := &regions.output[community]
		region.Members++

		if !grid.Present[row][column] || grid.baselines[row][column] == nil {
			continue
		}
		energy := grid.activations[row][column] * grid.activations[row][column]
		orientation := grid.graph[regions.anchors[community]][column].orientation()
		region.Strength += energy
		region.Authority += energy * grid.qualities[row][column]
		region.Level += energy * orientation * grid.baselines[row][column].Reading.ZScore
		region.Change += energy * orientation * grid.activations[row][column]
	}

	for index := range regions.output {
		region := &regions.output[index]

		if region.Strength > 0 {
			region.Authority /= region.Strength
			region.Level /= region.Strength
			region.Change /= region.Strength
		}
		region.Condition = ConditionToken(region.ID, region.Level, region.Change)
	}
	return regions.active(), grid.versions[row], nil
}

/* active selects the stronger activation class without changing membership. */
func (regions *regions) active() []Region {
	regions.output = slices.DeleteFunc(regions.output, func(region Region) bool {
		return region.Strength == 0
	})
	slices.SortFunc(regions.output, func(left, right Region) int {
		if left.Strength != right.Strength {
			return -int(math.Copysign(1, left.Strength-right.Strength))
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
	return regions.output[:keep]
}
