package fluid

/*
WireRow maps one symbol field reading to the dashboard wire shape.
*/
func WireRow(row SymbolSnapshot) map[string]any {
	return map[string]any{
		"symbol":     row.Symbol,
		"change_pct": row.ChangePct,
		"vol":        row.Vol,
		"div":        row.Div,
		"vort":       row.Vort,
		"turb":       row.Turb,
		"visc":       row.Visc,
		"re":         row.Re,
	}
}

/*
WireAggregate maps cross-section fluid scalars to the dashboard wire shape.
*/
func WireAggregate(aggregate FieldAggregate) map[string]any {
	return map[string]any{
		"div":  aggregate.Div,
		"vort": aggregate.Vort,
		"turb": aggregate.Turb,
		"visc": aggregate.Visc,
		"re":   aggregate.Re,
	}
}

/*
WireGrid maps one fluid terrain grid to the dashboard wire shape.
*/
func WireGrid(grid FluidGrid) map[string]any {
	return map[string]any{
		"size":         grid.Size,
		"heights":      grid.Heights,
		"min":          grid.Min,
		"max":          grid.Max,
		"filled_cells": grid.FilledCells,
		"outliers": map[string]any{
			"clipped_count":  grid.Outliers.ClippedCount,
			"clipped_at":     grid.Outliers.ClippedAt,
			"raw_max":        grid.Outliers.RawMax,
			"raw_max_symbol": grid.Outliers.RawMaxSymbol,
			"display_max":    grid.Outliers.DisplayMax,
		},
	}
}

/*
WireDisplay maps fluid presentation parameters to the dashboard wire shape.
*/
func WireDisplay(snapshot DisplayParamsSnapshot) map[string]any {
	return map[string]any{
		"height_ema_alpha": snapshot.HeightEMAAlpha,
		"grid_size":        snapshot.GridSize,
		"quantile_clip":    snapshot.QuantileClip,
	}
}
