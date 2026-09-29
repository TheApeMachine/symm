package impulse

import "github.com/theapemachine/symm/nomagique/learning/associative/grid"

/*
Snapshot materializes a visualization only when a Tee accepts this market.
The workspace calls it synchronously before releasing the producer boundary;
the asynchronous encoder never dereferences live owners. It is not history or
training input. Historical states are reconstructed from the ordered tape.
*/
func (market *Market) Snapshot() *grid.Snapshot {
	snapshot := &grid.Snapshot{
		Label: market.Symbol, Sequence: market.Sequence, Volume: market.Volume.String(),
		Cells:   make([]grid.Quantity, len(market.Cells)),
		Regions: make([]grid.Region, 0, len(market.regions)),
	}

	for _, region := range market.regions {
		if region.Members > 0 {
			snapshot.Regions = append(snapshot.Regions, region)
		}
	}

	if len(snapshot.Regions) == 0 {
		snapshot.Regions = append(snapshot.Regions, market.Impulse.Regions...)
	}

	for index, cell := range market.Cells {
		value, exists := cell.Value()
		snr := 0.0
		snrDefined := cell.owner != nil && cell.owner.measurement != nil && cell.owner.measurement.SNRDefined

		if snrDefined {
			snr = cell.owner.measurement.SNR
		}

		if !snrDefined && cell.Baseline.Count > 1 && cell.Baseline.M2 > 0 {
			snr = cell.Level * cell.Level
		}

		basinID := cell.ID

		if cell.Position.Basin >= 0 && cell.Position.Basin < len(market.Cells) {
			basinID = market.Cells[cell.Position.Basin].ID
		}

		snapshot.Cells[index] = grid.Quantity{ID: cell.ID, Source: cell.Owner, Label: cell.Metric,
			X: cell.Position.X, Y: cell.Position.Y, Value: value, Activity: cell.Activity,
			Quality: snr, Present: cell.Present && exists, Basin: basinID}
	}

	return snapshot
}
