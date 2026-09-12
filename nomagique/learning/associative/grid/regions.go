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
	regions.membership = make([]int, len(grid.columns))
	counts := make([]int, len(grid.columns))

	for column := range regions.membership {
		regions.membership[column] = column
		counts[column] = 1
	}
	scores := make([]float64, len(grid.columns))
	const maxFormIterations = 100

	for iteration := 0; iteration < maxFormIterations; iteration++ {
		moved := false

		for column := range regions.membership {
			clear(scores)

			for peer, reading := range grid.graph[column] {
				if peer == column {
					continue
				}
				weight := reading.strength() * (grid.weights[column] * grid.weights[peer])

				if !reading.stable() {
					weight = -weight
				}

				scores[regions.membership[peer]] += weight
			}
			current := regions.membership[column]
			currentAffinity := 0.0

			if counts[current] > 1 {
				currentAffinity = scores[current] / float64(counts[current]-1)
			}
			best := current
			bestAffinity := currentAffinity

			for candidate, score := range scores {
				if candidate == current || counts[candidate] == 0 {
					continue
				}
				candidateAffinity := score / float64(counts[candidate])

				if candidateAffinity > bestAffinity && candidateAffinity > 0 {
					best = candidate
					bestAffinity = candidateAffinity
				}
			}

			if best != current {
				counts[current]--
				counts[best]++
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

/* regionsOf measures the active region projection of one context. */
func (op *Space) regionsOf(label string) ([]Region, uint64, error) {
	op.mu.Lock()
	defer op.mu.Unlock()

	return op.regionsLocked(label)
}

func (op *Space) regionsLocked(label string) ([]Region, uint64, error) {
	row, exists := op.rowIndex[label]

	if !exists {
		return nil, 0, errnie.Error(errnie.Err(errnie.NotFound, "grid: unknown context "+label, nil))
	}

	if !op.formed {
		return nil, op.versions[row], nil
	}
	held := &op.regions
	held.output = slices.Grow(held.output[:0], len(held.anchors))[:len(held.anchors)]

	for index, anchor := range held.anchors {
		held.output[index] = Region{ID: uint64(anchor + 1)}
	}

	for column, community := range held.membership {
		if community < 0 || community >= len(held.output) {
			return nil, 0, errnie.Error(errnie.Err(
				errnie.Internal,
				"grid: region membership does not match the formed partition",
				nil,
			))
		}
		region := &held.output[community]
		region.Members++

		if !op.present[row][column] || op.baselines[row][column] == nil {
			continue
		}
		energy := op.activations[row][column] * op.activations[row][column]
		orientation := op.graph[held.anchors[community]][column].orientation()
		region.Strength += energy
		region.Authority += energy * op.qualities[row][column]
		region.Level += energy * orientation * op.zscores[row][column]
		region.Change += energy * orientation * op.activations[row][column]
	}

	for index := range held.output {
		region := &held.output[index]

		if region.Strength > 0 {
			region.Authority /= region.Strength
			region.Level /= region.Strength
			region.Change /= region.Strength
		}
		token, err := conditionToken(region.ID, region.Level, region.Change)

		if err != nil {
			return nil, 0, err
		}
		region.Condition = token
	}
	return held.active(), op.versions[row], nil
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
	count := len(regions.output)

	if count <= 1 {
		return regions.output
	}
	total := 0.0

	for _, region := range regions.output {
		total += region.Strength
	}

	if total <= 0 {
		return regions.output
	}
	bestCutoff := count
	maxVariance := 0.0
	sumFirst := 0.0

	for cutoff := 1; cutoff < count; cutoff++ {
		sumFirst += regions.output[cutoff-1].Strength
		meanFirst := sumFirst / float64(cutoff)
		meanRest := (total - sumFirst) / float64(count-cutoff)
		delta := meanFirst - meanRest
		variance := float64(cutoff*(count-cutoff)) * delta * delta

		if variance > maxVariance {
			maxVariance = variance
			bestCutoff = cutoff
		}
	}

	return regions.output[:bestCutoff]
}
