package ui

import (
	"time"

	"github.com/theapemachine/symm/fluid"
)

func symbolRowPayload(row fluid.SymbolSnapshot) map[string]any {
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

func fieldAggregatePayload(aggregate fluid.FieldAggregate) map[string]any {
	return map[string]any{
		"div":  aggregate.Div,
		"vort": aggregate.Vort,
		"turb": aggregate.Turb,
		"visc": aggregate.Visc,
		"re":   aggregate.Re,
	}
}

func fluidGridPayload(grid fluid.FluidGrid) map[string]any {
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
PublishFieldRow streams one symbol fluid reading to the dashboard.
*/
func (stream *MarketStream) PublishFieldRow(row fluid.SymbolSnapshot) {
	if stream == nil || row.Symbol == "" {
		return
	}

	stream.send(map[string]any{
		"event":  "field_row",
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"symbol": row.Symbol,
		"row":    symbolRowPayload(row),
	})
}

/*
PublishFieldAggregate streams cross-section fluid scalars after one symbol update.
*/
func (stream *MarketStream) PublishFieldAggregate(
	sampledCount int,
	aggregate fluid.FieldAggregate,
) {
	if stream == nil {
		return
	}

	stream.send(map[string]any{
		"event":        "field_aggregate",
		"ts":           time.Now().UTC().Format(time.RFC3339Nano),
		"symbol_count": sampledCount,
		"field":        fieldAggregatePayload(aggregate),
	})
}

/*
PublishFieldGrid streams the rebuilt fluid terrain grid for one scan pass.
*/
func (stream *MarketStream) PublishFieldGrid(grid fluid.FluidGrid) {
	if stream == nil {
		return
	}

	stream.send(map[string]any{
		"event": "field_grid",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"grid":  fluidGridPayload(grid),
	})
}

/*
FluidDisplay publishes the active fluid terrain presentation parameters.
*/
func (stream *MarketStream) FluidDisplay(snapshot fluid.DisplayParamsSnapshot) {
	if stream == nil {
		return
	}

	stream.send(map[string]any{
		"event":            "fluid_display",
		"ts":               time.Now().UTC().Format(time.RFC3339Nano),
		"height_ema_alpha": snapshot.HeightEMAAlpha,
		"grid_size":        snapshot.GridSize,
		"quantile_clip":    snapshot.QuantileClip,
	})
}
