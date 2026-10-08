package hawkes

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

var outputKeys = []string{
	"event_count",
	"event_count:buy",
	"event_count:sell",
	"event_fraction:buy",
	"event_fraction:sell",
	"arrival_rate:buy",
	"arrival_rate:sell",
	"arrival_rate",
	"conditional_intensity:buy",
	"conditional_intensity:sell",
	"conditional_intensity",
	"background_rate:buy",
	"background_rate:sell",
	"background_rate",
	"excitation_intensity:buy",
	"excitation_intensity:sell",
	"excitation_fraction:buy",
	"excitation_fraction:sell",
	"excitation_amplitude:buy_from_buy",
	"excitation_amplitude:buy_from_sell",
	"excitation_amplitude:sell_from_buy",
	"excitation_amplitude:sell_from_sell",
	"excitation_decay",
	"excitation_decay:buy_from_buy",
	"excitation_decay:buy_from_sell",
	"excitation_decay:sell_from_buy",
	"excitation_decay:sell_from_sell",
	"excitation_timescale",
	"excitation_timescale:buy_from_buy",
	"excitation_timescale:buy_from_sell",
	"excitation_timescale:sell_from_buy",
	"excitation_timescale:sell_from_sell",
	"offspring:buy_from_buy",
	"offspring:buy_from_sell",
	"offspring:sell_from_buy",
	"offspring:sell_from_sell",
	"branching_spectral_radius",
	"expected_descendants_from_buy",
	"expected_descendants_from_sell",
	"log_likelihood:hawkes",
	"log_likelihood_per_event:hawkes",
	"log_likelihood:poisson",
	"log_likelihood_gain_vs_poisson",
	"log_likelihood_gain_per_event_vs_poisson",
	"log_likelihood:self_only",
	"log_likelihood_gain_vs_self_only",
	"log_likelihood_gain_per_event_vs_self_only",
	"compensator:buy",
	"compensator:sell",
	"count_innovation:buy",
	"count_innovation:sell",
	"standardized_innovation:buy",
	"standardized_innovation:sell",
	"excitation_mass:buy",
	"excitation_mass:sell",
	"excitation_share:buy",
	"excitation_share:sell",
	"excitation_share",
	"snr",
	"historical_path_distance",
	"historical_path_percentile",
}

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

	res, err := model.Step(mark, atSec)

	if err != nil {
		signal.Error(errnie.Err(
			errnie.Internal,
			"[hawkes] "+prior.Label+": step failed",
			err,
		))

		return nil
	}

	output := make(map[string]float64, len(outputKeys))

	for i, key := range outputKeys {
		if i < len(res) {
			output[key] = res[i]
		}
	}

	if len(res) > len(outputKeys) {
		fromSec := res[len(outputKeys)]

		if fromSec > 0 {
			prior.From = time.Unix(0, int64(fromSec*1e9))
		}
	}

	return prior.Next(signal.Name(), output)
}
