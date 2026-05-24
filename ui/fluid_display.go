package ui

import "github.com/theapemachine/symm/fluid"

/*
FluidDisplayController applies UI display patches to the fluid signal.
*/
type FluidDisplayController interface {
	ApplyDisplayPatch(fluid.DisplayPatch) (fluid.DisplayParamsSnapshot, error)
	DisplayParams() fluid.DisplayParamsSnapshot
}

func fluidDisplayEvent(snapshot fluid.DisplayParamsSnapshot) map[string]any {
	return map[string]any{
		"event":            "fluid_display",
		"height_ema_alpha": snapshot.HeightEMAAlpha,
		"grid_size":        snapshot.GridSize,
		"quantile_clip":    snapshot.QuantileClip,
	}
}
