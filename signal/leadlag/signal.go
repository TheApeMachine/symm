package leadlag

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

var leadLagPeerFactKeys = []string{
	"reference_symbol",
	"contemporaneous_correlation",
	"best_lag_correlation",
	"best_lag_seconds",
	"best_lag_index",
	"absolute_correlation_gain",
	"lag_search_resolution_seconds",
	"lag_search_span",
	"lag_fraction",
	"reference_return_count",
	"measured_return_count",
	"overlap_pair_count",
	"search_count",
	"effective_sample_count",
	"lag_peak_prominence",
	"lag_peak_curvature",
	"correlation_p_value",
	"search_adjusted_p_value",
	"lag_baseline_seconds",
	"lag_divergence_seconds",
	"lag_noise_scale_seconds",
	"lag_zscore",
	"best_lag_correlation_baseline",
	"best_lag_correlation_zscore",
	"correlation_gain_baseline",
	"correlation_gain_zscore",
	"lag_velocity",
	"correlation_gain_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

/*
Signal is the asynchronous price-path lead-lag instrument. It holds no logic
of its own: its entire behavior is one nomagique Stages pipeline per symbol
over a shared output map. Member admits the arrival into the symbol's retained
path and publishes that path into the signal's shared path store; Leads runs
the exact discrete lag search of the symbol against every peer path in the
store. Peers never live on the Measurement: the keyed path store is the only
place symbols meet. Pair facts are published as "<fact>@<reference>"; a
positive lag means the reference's changes precede the measured symbol's.
Facts accumulate in the output map and are written once.
*/
type Signal struct {
	*runtime.System
	paths    *nmcorrelation.PathStore
	pipeline core.Primitive
}

func NewSignal(ctx context.Context) *Signal {
	paths := nmcorrelation.NewPathStore()
	signal := &Signal{
		paths: paths,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"pathstore", store.NewKV(),
				nomagique.NewNumber(
					nmcorrelation.NewMember("", paths),
					nmcorrelation.NewLeads("", paths, algo.NewHayashiYoshida()),
				),
				data.NewMessage(
					data.WRITE,
					"pathstore",
					"leadlag_state",
					data.NewValue[core.Primitive](),
				),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "leadlag", signal)
	return signal
}

/*
Step binds the arriving trade to the symbol's pipeline and writes the
published facts into a fresh Measurement allocated from the signal's own
arena. A trade frame that cannot be read or lacks price is an error; a trade
without a positive, finite price yields no measurement. Facts a stage left unwritten, or that are not finite, are
omitted, never fabricated as zero.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	var price float64
	var found bool
	for entry := range prior.Read("price") {
		if entry != nil && entry.Metric != nil {
			price = entry.Metric.Raw
			found = true
			break
		}
	}

	if !found {
		signal.Error(errnie.Err(errnie.Validation, "[leadlag] missing price", nil))
		return nil
	}

	if price <= 0 {
		return nil
	}

	output := make(map[string]float64)
	output["last"] = price
	atNano := float64(prior.At.UnixNano())
	signal.paths.SetCurrent(prior.Label)
	peers := signal.paths.Peers(prior.Label)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"pathstore",
			prior.Label,
			data.NewValue(price, atNano),
		).Next(nil),
	) {
		if ptr == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[signal.leadlag] pipeline returned nil",
				nil,
			))

			return nil
		}

		peerIdx := index / len(leadLagPeerFactKeys)
		keyIdx := index % len(leadLagPeerFactKeys)
		if peerIdx < len(peers) {
			output[leadLagPeerFactKeys[keyIdx]+"@"+peers[peerIdx]] = *(*float64)(ptr)
		}
		index++
	}

	if err := signal.pipeline.Error(); err != nil {
		signal.Error(err)
		return nil
	}

	return prior.Next(signal.Name(), output)
}
