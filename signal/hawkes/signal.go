package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
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
	last   map[string]*data.Measurement
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		models: make(map[string]*symbolModel),
		last:   make(map[string]*data.Measurement),
	}

	signal.System = runtime.NewSystem(ctx, "hawkes", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		return nil
	}

	if prior.Source != "spot:trade" {
		signal.Error(errnie.Err(
			errnie.NotAcceptable,
			"[hawkes] non-trade frame "+prior.Source+" reached a trade-only signal",
			nil,
		))

		return nil
	}

	for _, key := range []string{"price", "qty"} {
		found := false

		for entry := range prior.Read(key) {
			if entry != nil && entry.Metric != nil {
				found = true
				break
			}
		}

		if !found {
			signal.Error(errnie.Err(
				errnie.Validation, "[hawkes] trade frame missing "+key, nil,
			))
			return nil
		}
	}

	var side string

	if entry := data.Pull(prior.Meta("side")); entry != nil {
		side = entry.Value
	}

	var mark float64

	if side == "buy" {
		mark = 1.0
	}

	if side == "sell" {
		mark = -1.0
	}

	if side != "buy" && side != "sell" {
		if prior.Source == "spot:trade" {
			errnie.Warn(signal.Name() + ": trade without an explicit aggressor side; dropping event")
		}

		return nil
	}

	atSec := float64(prior.At.UnixNano()) * 1e-9

	state, exists := signal.models[prior.Label]

	if !exists {
		state = &symbolModel{
			estimator: nmhawkes.NewHawkes(),
		}
		signal.models[prior.Label] = state
	}

	res, from, err := state.estimator.Step(mark, atSec)

	if err != nil {
		signal.Error(errnie.Err(
			errnie.Internal,
			"[hawkes] "+prior.Label+": step failed",
			err,
		))

		return nil
	}

	if dropped := state.estimator.Dropped(); dropped != nil {
		errnie.Warn("[hawkes] " + prior.Label + ": model dropped: " + dropped.Error())
	}

	outMetrics := make(map[string]float64, 4)

	if buyRate, ok := res["arrival_rate:buy"]; ok {
		outMetrics["arrival_rate:buy"] = buyRate
	}

	if sellRate, ok := res["arrival_rate:sell"]; ok {
		outMetrics["arrival_rate:sell"] = sellRate
	}

	if radius, ok := res["branching_spectral_radius"]; ok {
		outMetrics["branching_spectral_radius"] = radius
	}

	if intensity, ok := res["conditional_intensity"]; ok {
		if state.hasPrev {
			timeDelta := atSec - state.prevAtSec

			if core.Resolvable(timeDelta) {
				outMetrics["conditional_intensity_velocity"] = (intensity - state.prevIntensity) / timeDelta
			}
		}

		state.prevIntensity = intensity
		state.prevAtSec = atSec
		state.hasPrev = true
	}

	sideEntry := &data.StringEntry{Key: "side", Value: side}

	prev := signal.last[prior.Label]
	if prev == nil {
		prev = data.NewMeasurement(prior.Epoch, prior.Label, signal.Name(), prior.At, prior.From)
	}

	out := prev.Next(prior, outMetrics, sideEntry)
	out.From = from
	signal.last[prior.Label] = out
	return out
}
