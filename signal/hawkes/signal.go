package hawkes

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the Hawkes arrival-dynamics instrument. It holds no estimation
state of its own: its entire behavior is one nomagique Stages pipeline over
a shared output map. The gate classifies the trade's side, the counts stage
admits the arrival into the symbol's observation window, the excitation
stage measures the arrival against the model fitted before it, and the
refit stage folds the arrival into the history and re-estimates for the
next one. Per-symbol arrival paths and fitted models live inside the
pipeline's shared stage registry.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	metrics   [][4]string
}

type symbolPipeline struct {
	output   data.Map[float64]
	envelope data.Map[float64]
	state    *data.State
	pipeline core.Primitive
}

/*
NewSignal composes the arrival-dynamics instrument.
*/
func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	signal := &Signal{
		arena: arena,
		// {published label, output key, unit, timescale}
		metrics: [][4]string{
			{"event_count", "event_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"event_count:buy", "event_count:buy", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"event_count:sell", "event_count:sell", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"event_fraction:buy", "event_fraction:buy", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"event_fraction:sell", "event_fraction:sell", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"arrival_rate:buy", "arrival_rate:buy", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"arrival_rate:sell", "arrival_rate:sell", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"arrival_rate", "arrival_rate", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"conditional_intensity:buy", "conditional_intensity:buy", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"conditional_intensity:sell", "conditional_intensity:sell", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"conditional_intensity", "conditional_intensity", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"background_rate:buy", "background_rate:buy", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"background_rate:sell", "background_rate:sell", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"background_rate", "background_rate", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_intensity:buy", "excitation_intensity:buy", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_intensity:sell", "excitation_intensity:sell", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_fraction:buy", "excitation_fraction:buy", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"excitation_fraction:sell", "excitation_fraction:sell", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"excitation_amplitude:buy_from_buy", "excitation_amplitude:buy_from_buy", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_amplitude:buy_from_sell", "excitation_amplitude:buy_from_sell", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_amplitude:sell_from_buy", "excitation_amplitude:sell_from_buy", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_amplitude:sell_from_sell", "excitation_amplitude:sell_from_sell", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_decay", "excitation_decay", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_decay:buy_from_buy", "excitation_decay:buy_from_buy", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_decay:buy_from_sell", "excitation_decay:buy_from_sell", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_decay:sell_from_buy", "excitation_decay:sell_from_buy", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_decay:sell_from_sell", "excitation_decay:sell_from_sell", string(data.UnitRate), string(data.TimescaleInstantaneous)},
			{"excitation_timescale", "excitation_timescale", string(data.UnitDuration), string(data.TimescaleInstantaneous)},
			{"excitation_timescale:buy_from_buy", "excitation_timescale:buy_from_buy", string(data.UnitDuration), string(data.TimescaleInstantaneous)},
			{"excitation_timescale:buy_from_sell", "excitation_timescale:buy_from_sell", string(data.UnitDuration), string(data.TimescaleInstantaneous)},
			{"excitation_timescale:sell_from_buy", "excitation_timescale:sell_from_buy", string(data.UnitDuration), string(data.TimescaleInstantaneous)},
			{"excitation_timescale:sell_from_sell", "excitation_timescale:sell_from_sell", string(data.UnitDuration), string(data.TimescaleInstantaneous)},
			{"offspring:buy_from_buy", "offspring:buy_from_buy", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"offspring:buy_from_sell", "offspring:buy_from_sell", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"offspring:sell_from_buy", "offspring:sell_from_buy", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"offspring:sell_from_sell", "offspring:sell_from_sell", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"branching_spectral_radius", "branching_spectral_radius", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"expected_descendants_from_buy", "expected_descendants_from_buy", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"expected_descendants_from_sell", "expected_descendants_from_sell", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"log_likelihood:hawkes", "log_likelihood:hawkes", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"log_likelihood_per_event:hawkes", "log_likelihood_per_event:hawkes", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"log_likelihood:poisson", "log_likelihood:poisson", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"log_likelihood_gain_vs_poisson", "log_likelihood_gain_vs_poisson", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"log_likelihood_gain_per_event_vs_poisson", "log_likelihood_gain_per_event_vs_poisson", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"log_likelihood:self_only", "log_likelihood:self_only", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"log_likelihood_gain_vs_self_only", "log_likelihood_gain_vs_self_only", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"log_likelihood_gain_per_event_vs_self_only", "log_likelihood_gain_per_event_vs_self_only", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"compensator:buy", "compensator:buy", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"compensator:sell", "compensator:sell", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"count_innovation:buy", "count_innovation:buy", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"count_innovation:sell", "count_innovation:sell", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"standardized_innovation:buy", "standardized_innovation:buy", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"standardized_innovation:sell", "standardized_innovation:sell", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"excitation_mass:buy", "excitation_mass:buy", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"excitation_mass:sell", "excitation_mass:sell", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"excitation_share:buy", "excitation_share:buy", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"excitation_share:sell", "excitation_share:sell", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"excitation_share", "excitation_share", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"snr", "snr", string(data.UnitSNR), string(data.TimescaleInstantaneous)},
		},
	}

	signal.System = runtime.NewSystem(ctx, "hawkes", signal)
	return signal
}

func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

func (signal *Signal) pipelineFor(symbol string) *symbolPipeline {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(*symbolPipeline)
	}

	output := data.NewOutputMap()
	history := nmhawkes.Paths()

	pipe := &symbolPipeline{
		output:   output,
		envelope: data.NewOutputMap(),
		state:    data.NewState(data.NewMap(), output),
		pipeline: transport.NewStages(
			nmhawkes.NewGate(),
			nmhawkes.NewCounts(history, symbol),
			nmhawkes.NewExcitation(history, symbol),
			nmhawkes.NewRefit(history, symbol),
		),
	}

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipe)
	return actual.(*symbolPipeline)
}

/*
Step binds the prior trade Measurement to one adapter, runs the sequential
hawkes Stages pipeline, and writes the published facts into a fresh
Measurement allocated from the signal's own arena. Facts a stage left
unwritten are omitted, never fabricated as zero.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	if prior.Meta("channel") != "trade" {
		return nil
	}

	price := data.Pull(prior.Read("price"))
	qty := data.Pull(prior.Read("qty"))

	if price.Err != nil || qty.Err != nil || price.Metric.Label == "" || qty.Metric.Label == "" {
		return nil
	}

	pipe := signal.pipelineFor(prior.Label)

	clear(pipe.output.Values)
	clear(pipe.envelope.Values)

	switch prior.Meta("side") {
	case "buy":
		pipe.envelope.Values["buy"] = 1
		pipe.envelope.Values["sell"] = 0
	case "sell":
		pipe.envelope.Values["buy"] = 0
		pipe.envelope.Values["sell"] = 1
	default:
		errnie.Warn(signal.Name() + ": trade without an explicit aggressor side; dropping event")
		return nil
	}

	pipe.envelope.Values["at"] = float64(prior.At.UnixNano()) * 1e-9

	adapter := data.NewAdapter(prior, pipe.state)

	for range adapter.Next(data.NewValue(pipe.envelope)) {
	}

	for range pipe.pipeline.Next(data.NewValue(adapter)) {
	}

	if err := errors.Join(adapter.Error(), pipe.pipeline.Error()); err != nil {
		signal.Error(err)
		return nil
	}

	out := signal.arena.NewMeasurement(
		prior.Epoch, prior.Label, signal.Name(), prior.SeqIdx, prior.Tick, []*data.Measurement{prior},
	)
	out.Epoch = prior.Epoch
	out.Label = prior.Label
	out.Source = signal.Name()
	out.SeqIdx = prior.SeqIdx
	out.Tick = prior.Tick
	out.At = prior.At
	out.From = prior.At

	if fromSec, held := pipe.output.Values["from"]; held {
		out.From = time.Unix(0, int64(fromSec*1e9)).UTC()
	}

	metrics := make([]data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value, held := pipe.output.Values[metric[1]]

		if !held {
			continue
		}

		metrics = append(metrics, data.NewMetric(
			metric[0], value, data.Unit(metric[2]), data.Timescale(metric[3]),
		))
	}

	return out.Write(metrics...)
}
