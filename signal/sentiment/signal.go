package sentiment

import (
	"context"
	"errors"
	"math"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/crosssection"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the cross-sectional price-state instrument. It holds no logic of
its own: its entire behavior is two nomagique pipelines over one shared
output map and one shared member store. The cohort pipeline retains the
focal member's price, derives its change, and reduces the cohort's changes
to sign counts, signed breadth, the median change, and breadth's causal
baseline. The derived pipeline, a transport.Parallel of stage groups, runs
only once the cohort holds at least one member change, and derives the
participation fractions, the median change's causal baseline, and the
velocities. The quoted price and member identity are the only envelope
translation.
*/
type Signal struct {
	*runtime.System
	arena    *data.ArenaOwner
	output   data.Map[float64]
	envelope data.Map[float64]
	identity data.Map[string]
	cohort   *data.State
	states   []*data.State
	members  core.Primitive
	reduce   core.Primitive
	pipeline core.Primitive
	metrics  [][5]string
}

/*
NewSignal composes the cross-sectional price-state instrument.
*/
func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	output := data.NewOutputMap()
	members := store.NewKV[string, float64](nil)

	signal := &Signal{
		arena:    arena,
		output:   output,
		envelope: data.NewOutputMap(),
		identity: data.NewTextMap(),
		members:  members,
		// The cohort state binds the crosssection reductions' native names to
		// sentiment domain names.
		cohort: data.NewState(data.NewMap(
			"positive_count", "advance_count",
			"negative_count", "decline_count",
			"zero_count", "unchanged_count",
			"signed_fraction", "breadth",
			"signed_median", "median_return",
			"signed_fraction_baseline", "breadth_baseline",
			"signed_fraction_divergence", "breadth_divergence",
			"signed_fraction_zscore", "breadth_zscore",
		), output),
		reduce: transport.NewStages(
			crosssection.NewUpdateMember("price", members),
			crosssection.NewChangeCounts(members),
			crosssection.NewChangeMedian(members),
			crosssection.NewChangeBaseline(),
		),
		states: []*data.State{
			// 0-2: Participation fractions.
			data.NewState(data.NewMap("left", "advance_count", "right", "valid_member_count", "divide", "advance_fraction"), output),
			data.NewState(data.NewMap("left", "decline_count", "right", "valid_member_count", "divide", "decline_fraction"), output),
			data.NewState(data.NewMap("left", "unchanged_count", "right", "valid_member_count", "divide", "unchanged_fraction"), output),
			// 3-5: Causal baseline of the median change, divergence, z-score.
			data.NewState(data.NewMap("value", "median_return", "center", "median_return_baseline", "scale", "median_return_scale"), output),
			data.NewState(data.NewMap("left", "median_return", "right", "median_return_baseline", "subtract", "median_return_divergence"), output),
			data.NewState(data.NewMap("left", "median_return_divergence", "right", "median_return_scale", "divide", "median_return_zscore"), output),
			// 6-7: Velocities of the median change and of breadth.
			data.NewState(data.NewMap("value", "median_return", "rate", "median_return_velocity", "defined", "median_return_velocity:defined"), output),
			data.NewState(data.NewMap("value", "breadth", "rate", "breadth_velocity", "defined", "breadth_velocity:defined"), output),
		},
		pipeline: transport.NewParallel(
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
			transport.NewStages(temporal.NewVelocity()),
		),
		// {published label, output key, unit, timescale, gate key}
		// A non-empty gate key publishes the metric only while that output is non-zero.
		metrics: [][5]string{
			{"valid_member_count", "valid_member_count", string(data.UnitCount), string(data.TimescaleInstantaneous), ""},
			{"advance_count", "advance_count", string(data.UnitCount), string(data.TimescaleInstantaneous), ""},
			{"decline_count", "decline_count", string(data.UnitCount), string(data.TimescaleInstantaneous), ""},
			{"unchanged_count", "unchanged_count", string(data.UnitCount), string(data.TimescaleInstantaneous), ""},
			{"advance_fraction", "advance_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"decline_fraction", "decline_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"unchanged_fraction", "unchanged_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"breadth", "breadth", string(data.UnitDimensionless), string(data.TimescaleInstantaneous), ""},
			{"median_return", "median_return", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"breadth_baseline", "breadth_baseline", string(data.UnitDimensionless), string(data.TimescaleRollingWindow), ""},
			{"breadth_divergence", "breadth_divergence", string(data.UnitDimensionless), string(data.TimescaleRollingWindow), ""},
			{"breadth_zscore", "breadth_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), ""},
			{"median_return_baseline", "median_return_baseline", string(data.UnitRatio), string(data.TimescaleRollingWindow), ""},
			{"median_return_divergence", "median_return_divergence", string(data.UnitRatio), string(data.TimescaleRollingWindow), ""},
			{"median_return_zscore", "median_return_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), ""},
			{"median_return_velocity", "median_return_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous), "median_return_velocity:defined"},
			{"breadth_velocity", "breadth_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous), "breadth_velocity:defined"},
		},
	}

	signal.System = runtime.NewSystem(ctx, "sentiment", signal)
	return signal
}

/*
Arena exposes the signal's ArenaOwner to the runtime Consumer.
*/
func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

/*
Step binds the prior quote Measurement to the cohort adapter, runs the cohort
reduction, runs the derived pipeline once the cohort holds a member change,
and writes the published facts into a fresh Measurement allocated from the
signal's own arena. A quote without a finite positive price yields no
measurement. Facts a stage left unwritten are omitted, never fabricated as
zero.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	clear(signal.output.Values)
	clear(signal.envelope.Values)
	clear(signal.identity.Values)

	price, err := tradeValue(prior, "price")

	if err != nil {
		signal.Error(err)
		return nil
	}

	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return nil
	}

	signal.envelope.Values["price"] = price

	signal.envelope.Values["at"] = float64(prior.At.UnixNano())
	signal.identity.Values["member"] = prior.Label

	cohort := data.NewAdapter(prior, signal.cohort)

	for range cohort.Next(data.NewValue(signal.envelope)) {
	}

	for range cohort.Next(data.NewValue(signal.identity)) {
	}

	for range signal.reduce.Next(data.NewValue(cohort)) {
	}

	if err := errors.Join(cohort.Error(), signal.reduce.Error()); err != nil {
		signal.Error(err)
		return nil
	}

	if signal.output.Values["valid_member_count"] > 0 {
		adapters := make([]*data.Adapter, len(signal.states))

		for index, state := range signal.states {
			adapters[index] = data.NewAdapter(prior, state)
		}

		for range signal.pipeline.Next(data.NewValue(adapters...)) {
		}

		if err := signal.pipeline.Error(); err != nil {
			signal.Error(err)
			return nil
		}
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

	metrics := make([]*data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value, held := signal.output.Values[metric[1]]

		if !held {
			continue
		}

		if metric[4] != "" && signal.output.Values[metric[4]] == 0 {
			continue
		}

		metrics = append(metrics, data.NewMetric(
			metric[0], value, data.Unit(metric[2]), data.Timescale(metric[3]),
		))
	}

	return out.Write(metrics...)
}

/*
tradeValue reads one required trade field from a trade frame. A read failure
or an absent field is an error: every trade frame carries price and qty, so a
frame without them is broken upstream and must not be silently skipped.
*/
func tradeValue(prior *data.Measurement, key string) (float64, error) {
	entry := data.Pull(prior.Read(key))

	if entry != nil && entry.Err != nil {
		return 0, entry.Err
	}

	if entry == nil || entry.Metric == nil || entry.Metric.Label != key {
		return 0, errnie.Err(
			errnie.NotAcceptable, "[sentiment] trade frame is missing "+key, nil,
		)
	}

	return entry.Metric.Raw, nil
}
