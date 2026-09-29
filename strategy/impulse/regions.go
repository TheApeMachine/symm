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

	validRegions := market.regions[:0]

	for _, region := range market.regions {
		if region.Members == 0 {
			continue
		}

		if region.Strength > 0 {
			region.Authority /= region.Strength
			region.Level /= region.Strength
			region.Change /= region.Strength
		}

		condition, err := grid.Condition(region.ID, region.Level, region.Change)

		if err != nil {
			return err
		}

		region.Condition = condition
		validRegions = append(validRegions, region)
	}

	// Sort regions by strength descending (most lit up first)
	slices.SortFunc(validRegions, func(left, right grid.Region) int {
		if ordering := cmp.Compare(right.Strength, left.Strength); ordering != 0 {
			return ordering
		}
		return cmp.Compare(left.ID, right.ID)
	})

	var litRegions []grid.Region

	for _, region := range validRegions {
		if region.Strength > 0 {
			litRegions = append(litRegions, region)
		}
	}

	topN := len(litRegions)

	if len(litRegions) > 1 {
		total := 0.0
		for _, region := range litRegions {
			total += region.Strength
		}

		partial := 0.0
		bestVariance := 0.0
		cutoff := len(litRegions)

		for split := 1; split < len(litRegions); split++ {
			partial += litRegions[split-1].Strength
			weightLeft := float64(split)
			weightRight := float64(len(litRegions) - split)
			meanLeft := partial / weightLeft
			meanRight := (total - partial) / weightRight
			difference := meanLeft - meanRight
			variance := weightLeft * weightRight * difference * difference

			if variance > bestVariance {
				bestVariance = variance
				cutoff = split
			}
		}

		if bestVariance > 0 {
			topN = cutoff
		}
	}

	market.Impulse = grid.Impulse{Label: market.Symbol, SeqIdx: market.Sequence,
		Version: uint64(market.Sequence), Ready: market.valid, Regions: litRegions[:topN]}
	return nil
}
