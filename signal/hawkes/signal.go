package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
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
			"arrival_rate:buy",
			"arrival_rate:sell",
			"branching_spectral_radius",
			"conditional_intensity",
			"conditional_intensity_velocity",
		},
	}

	signal.System = runtime.NewSystem(ctx, "hawkes", signal)
	return signal
}

/*
Step processes every trade entry of the tick's frame; per-symbol state is keyed
by the entry's symbol and each entry writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	trades, err := data.AttributeOk[[]kraken.TradeData](value.Layers[0], "data")

	if err != nil {
		errnie.Error(err)
		return value
	}

	for _, trade := range trades {
		signal.step(value, trade)
	}

	return value
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	side := trade.Side
	var mark float64

	switch side {
	case "buy":
		mark = 1.0
	case "sell":
		mark = -1.0
	default:
		return value
	}

	price := trade.Price.Float64()
	qty := trade.Qty

	if price <= 0 || qty <= 0 {
		return value
	}

	atSec := float64(value.Timestamp) * 1e-9

	if atSec <= 0 {
		return value
	}

	state, exists := signal.models[trade.Symbol]

	if !exists {
		state = &symbolModel{
			estimator: nmhawkes.NewHawkes(),
		}
		signal.models[trade.Symbol] = state
	}

	res, _, err := state.estimator.Step(mark, atSec)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "[hawkes] estimator step failed for "+trade.Symbol, err))
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

	return data.AppendSignalVector(value, signal.Name(), trade.Symbol, []float64{
		buyRate,
		sellRate,
		radius,
		intensity,
		intensityVelocity,
	}, signal.keys...)
}
