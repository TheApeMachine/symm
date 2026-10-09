package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

type Signal struct {
	*runtime.System
	models map[string]*nmhawkes.Hawkes
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		models: make(map[string]*nmhawkes.Hawkes),
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

	side := prior.Meta("side")
	var mark float64

	if side == "buy" {
		mark = 1.0
	}

	if side == "sell" {
		mark = -1.0
	}

	if side != "buy" && side != "sell" {
		errnie.Warn(signal.Name() + ": trade without an explicit aggressor side; dropping event")
		return nil
	}

	atSec := float64(prior.At.UnixNano()) * 1e-9

	model, exists := signal.models[prior.Label]

	if !exists {
		model = nmhawkes.NewHawkes()
		signal.models[prior.Label] = model
	}

	res, from, err := model.Step(mark, atSec)

	if err != nil {
		signal.Error(errnie.Err(
			errnie.Internal,
			"[hawkes] "+prior.Label+": step failed",
			err,
		))

		return nil
	}

	out := prior.Next(signal.Name(), res)
	out.From = from

	return out
}
