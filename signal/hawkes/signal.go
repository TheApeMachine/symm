package hawkes

import (
	"context"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

type symbolModel struct {
	estimator     *nmhawkes.Hawkes
	prevIntensity float64
	prevAtSec     float64
	hasPrev       bool
}

type Signal struct {
	*runtime.System
	models map[string]*symbolModel
	keys   []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		models: make(map[string]*symbolModel),
		keys: []string{
			"arrival_rate_buy",
			"arrival_rate_sell",
			"branching_spectral_radius",
			"conditional_intensity",
			"conditional_intensity_velocity",
		},
	}

	signal.System = runtime.NewSystem(ctx, "hawkes", signal)
	return signal
}

func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	side := data.Attribute[string](value.Layers[0], "data", 0, "side")
	var mark float64
	if side == "buy" {
		mark = 1.0
	} else if side == "sell" {
		mark = -1.0
	} else {
		return value
	}

	price := data.Attribute[float64](value.Layers[0], "data", 0, "price")
	qty := data.Attribute[float64](value.Layers[0], "data", 0, "qty")
	if price <= 0 || qty <= 0 {
		return value
	}

	atSec := float64(value.Timestamp) * 1e-9
	if atSec <= 0 {
		return value
	}

	state, exists := signal.models[value.Scope]
	if !exists {
		state = &symbolModel{
			estimator: nmhawkes.NewHawkes(),
		}
		signal.models[value.Scope] = state
	}

	res, _, err := state.estimator.Step(mark, atSec)
	if err != nil {
		return value
	}

	buyRate := res["arrival_rate:buy"]
	sellRate := res["arrival_rate:sell"]
	radius := res["branching_spectral_radius"]
	intensity := res["conditional_intensity"]
	var intensityVelocity float64

	if intensity > 0 && state.hasPrev {
		timeDelta := atSec - state.prevAtSec
		if core.Resolvable(timeDelta) {
			intensityVelocity = (intensity - state.prevIntensity) / timeDelta
		}
	}

	if intensity > 0 {
		state.prevIntensity = intensity
		state.prevAtSec = atSec
		state.hasPrev = true
	}

	return data.AppendVector(value, []float64{
		buyRate,
		sellRate,
		radius,
		intensity,
		intensityVelocity,
	}, signal.keys...)
}
