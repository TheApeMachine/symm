package fluid

import (
	"fmt"
	"sync"

	"github.com/theapemachine/symm/config"
)

const (
	minHeightEMAAlpha = 0.05
	maxHeightEMAAlpha = 1.0
	minGridSize       = 8
	maxGridSize       = 64
	minQuantileClip   = 0.5
	maxQuantileClip   = 0.99
)

/*
DisplayParamsSnapshot is the active fluid grid presentation config.
*/
type DisplayParamsSnapshot struct {
	HeightEMAAlpha float64 `json:"height_ema_alpha"`
	GridSize       int     `json:"grid_size"`
	QuantileClip   float64 `json:"quantile_clip"`
}

/*
DisplayPatch carries optional fluid display overrides from UI control messages.
*/
type DisplayPatch struct {
	HeightEMAAlpha *float64 `json:"height_ema_alpha,omitempty"`
	GridSize       *int     `json:"grid_size,omitempty"`
	QuantileClip   *float64 `json:"quantile_clip,omitempty"`
	ResetSmoothing *bool    `json:"reset_smoothing,omitempty"`
}

/*
DisplayParams holds runtime fluid terrain presentation settings.
*/
type DisplayParams struct {
	mu             sync.RWMutex
	heightEMAAlpha float64
	gridSize       int
	quantileClip   float64
}

/*
NewDisplayParams returns defaults aligned with config.System.
*/
func NewDisplayParams() *DisplayParams {
	params := &DisplayParams{
		heightEMAAlpha: HeightEMAAlpha,
		gridSize:       GridSize,
		quantileClip:   gridQuantileClip,
	}

	params.applyConfigDefaults()

	return params
}

func (params *DisplayParams) applyConfigDefaults() {
	if config.System.FluidHeightEMAAlpha > 0 {
		params.heightEMAAlpha = config.System.FluidHeightEMAAlpha
	}

	if config.System.FluidGridSize > 0 {
		params.gridSize = config.System.FluidGridSize
	}

	if config.System.FluidQuantileClip > 0 {
		params.quantileClip = config.System.FluidQuantileClip
	}
}

/*
Apply merges one control patch and returns whether grid size changed.
*/
func (params *DisplayParams) Apply(patch DisplayPatch) (gridSizeChanged bool, err error) {
	params.mu.Lock()
	defer params.mu.Unlock()

	if patch.HeightEMAAlpha != nil {
		alpha := *patch.HeightEMAAlpha

		if alpha < minHeightEMAAlpha || alpha > maxHeightEMAAlpha {
			return false, fmt.Errorf(
				"height_ema_alpha out of range [%g, %g]",
				minHeightEMAAlpha,
				maxHeightEMAAlpha,
			)
		}

		params.heightEMAAlpha = alpha
	}

	if patch.GridSize != nil {
		size := *patch.GridSize

		if size < minGridSize || size > maxGridSize {
			return false, fmt.Errorf("grid_size out of range [%d, %d]", minGridSize, maxGridSize)
		}

		if size != params.gridSize {
			gridSizeChanged = true
		}

		params.gridSize = size
	}

	if patch.QuantileClip != nil {
		clip := *patch.QuantileClip

		if clip < minQuantileClip || clip > maxQuantileClip {
			return false, fmt.Errorf(
				"quantile_clip out of range [%g, %g]",
				minQuantileClip,
				maxQuantileClip,
			)
		}

		params.quantileClip = clip
	}

	return gridSizeChanged, nil
}

/*
Snapshot returns a copy of the active display parameters.
*/
func (params *DisplayParams) Snapshot() DisplayParamsSnapshot {
	params.mu.RLock()
	defer params.mu.RUnlock()

	return DisplayParamsSnapshot{
		HeightEMAAlpha: params.heightEMAAlpha,
		GridSize:       params.gridSize,
		QuantileClip:   params.quantileClip,
	}
}

func (params *DisplayParams) activeHeightEMAAlpha() float64 {
	params.mu.RLock()
	defer params.mu.RUnlock()

	if params.heightEMAAlpha <= 0 {
		return HeightEMAAlpha
	}

	return params.heightEMAAlpha
}

func (params *DisplayParams) activeGridSize() int {
	params.mu.RLock()
	defer params.mu.RUnlock()

	if params.gridSize <= 0 {
		return GridSize
	}

	return params.gridSize
}

func (params *DisplayParams) activeQuantileClip() float64 {
	params.mu.RLock()
	defer params.mu.RUnlock()

	if params.quantileClip <= 0 {
		return gridQuantileClip
	}

	return params.quantileClip
}
