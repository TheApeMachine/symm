package hawkes

import (
	"context"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
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
	pipeline *nomagique.Number
}

func NewSignal(ctx context.Context) *Signal {
	indices := make([]int, len(outputKeys)+1)
	for i := range indices {
		indices[i] = i
	}

	signal := &Signal{
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore", store.NewKV(),
				nomagique.NewNumber(
					data.NewValue[core.Primitive](
						nomagique.NewNumber(
							transport.NewSpread[float64](),
							nmhawkes.NewHawkes(),
						),
					),
					data.NewSelect(indices...),
				),
				data.NewMessage(data.WRITE, "symbolstore", "hawkes_state", data.NewValue[core.Primitive]()),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "hawkes", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if prior.Source != "spot:trade" {
		signal.Error(errnie.Err(
			errnie.NotAcceptable, "[hawkes] non-trade frame "+prior.Source+" reached a trade-only signal", nil,
		))
		return nil
	}

	for _, key := range []string{"price", "qty"} {
		if _, err := tradeValue(prior, key); err != nil {
			signal.Error(err)
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
	raws := []float64{mark, atSec}

	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(
				unsafe.Pointer(&raws),
			),
		).Next(nil),
	) {
		if ptr == nil {
			continue
		}

		if index < len(outputKeys) {
			output[outputKeys[index]] = *(*float64)(ptr)
		}

		if index == len(outputKeys) {
			fromSec := *(*float64)(ptr)
			if fromSec > 0 {
				prior.From = time.Unix(0, int64(fromSec*1e9))
			}
		}

		index++
	}

	return prior.Next(signal.Name(), output)
}

func tradeValue(prior *data.Measurement, key string) (float64, error) {
	entry := data.Pull(prior.Read(key))

	if entry != nil && entry.Err != nil {
		return 0, entry.Err
	}

	if entry == nil || entry.Metric == nil || entry.Metric.Label != key {
		return 0, errnie.Err(
			errnie.NotAcceptable, "[hawkes] trade frame is missing "+key, nil,
		)
	}

	return entry.Metric.Raw, nil
}
