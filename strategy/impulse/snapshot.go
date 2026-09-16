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
		Regions: append([]grid.Region(nil), market.Impulse.Regions...),
	}

	for index, cell := range market.Cells {
		value, exists := cell.Value()
		snapshot.Cells[index] = grid.Quantity{ID: cell.ID, Source: cell.Owner, Label: cell.Metric,
			X: cell.Position.X, Y: cell.Position.Y, Value: value, Activity: cell.Activity,
			Quality: cell.Position.Authority, Present: cell.Present && exists}
	}

	return snapshot
}
