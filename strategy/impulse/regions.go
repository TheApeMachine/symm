package impulse

import (
	"cmp"
	"slices"

	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/* light reduces current activity over the watershed basins, strongest first. */
func (market *Market) light() error {
	market.regions = market.regions[:len(market.Cells)]
	clear(market.regions)

	for index, cell := range market.Cells {
		basin := cell.Position.Basin
		region := &market.regions[basin]
		region.ID = market.Cells[basin].ID
		region.Members++
		region.Strength += cell.Activity
		region.Authority += cell.Activity * cell.Position.Authority
		orientation := 1.0

		if index != basin {
			left, right := min(index, basin), max(index, basin)
			orientation = market.pairs[right*(right-1)/2+left].Reading().Orientation
		}

		region.Level += cell.Activity * orientation * cell.Level
		region.Change += cell.Activity * orientation * cell.Movement
	}

	active := market.regions[:0]

	for _, region := range market.regions {
		if region.Strength == 0 {
			continue
		}

		region.Authority /= region.Strength
		region.Level /= region.Strength
		region.Change /= region.Strength
		condition, err := grid.Condition(region.ID, region.Level, region.Change)

		if err != nil {
			return err
		}

		region.Condition = condition
		active = append(active, region)
	}

	slices.SortFunc(active, func(left, right grid.Region) int {
		if ordering := cmp.Compare(right.Strength, left.Strength); ordering != 0 {
			return ordering
		}
		return cmp.Compare(left.ID, right.ID)
	})

	// Exact one-dimensional Otsu split over measured region energies.
	total, partial, bestVariance := 0.0, 0.0, 0.0
	cutoff := len(active)

	for _, region := range active {
		total += region.Strength
	}

	for split := 1; split < len(active); split++ {
		partial += active[split-1].Strength
		difference := partial/float64(split) - (total-partial)/float64(len(active)-split)
		variance := float64(split*(len(active)-split)) * difference * difference

		if variance > bestVariance {
			cutoff, bestVariance = split, variance
		}
	}

	market.Impulse = grid.Impulse{Label: market.Symbol, SeqIdx: market.Sequence,
		Version: uint64(market.Sequence), Ready: market.valid, Regions: active[:cutoff]}
	return nil
}
