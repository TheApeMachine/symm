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
	output   data.Map[float64]
	envelope data.Map[float64]
	identity data.Map[string]
	cohort   *data.State
	states   []*data.State
	members  core.Primitive
	reduce   core.Primitive
	pipeline core.Primitive
	metrics  [][4]string
}

/*
NewSignal composes the cross-sectional price-state instrument.
*/
func NewSignal(ctx context.Context) *Signal {
	output := data.NewOutputMap()
	members := store.NewKV[string, float64](nil)

	signal := &Signal{
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
		// {published label, output key, unit, timescale}
		metrics: [][4]string{
			{"cohort_member_count", "cohort_member_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"valid_member_count", "valid_member_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"excluded_member_count", "excluded_member_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"cohort_horizon_seconds", "cohort_horizon_seconds", string(data.UnitSecond), string(data.TimescaleInstantaneous)},
			{"return", "return", string(data.UnitLogReturn), string(data.TimescaleInstantaneous)},
			{"absolute_return", "absolute_return", string(data.UnitLogReturn), string(data.TimescaleInstantaneous)},
			{"asof_age_seconds", "asof_age_seconds", string(data.UnitSecond), string(data.TimescaleInstantaneous)},
			{"from_age_seconds", "from_age_seconds", string(data.UnitSecond), string(data.TimescaleInstantaneous)},
			{"advance_count", "advance_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"decline_count", "decline_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"unchanged_count", "unchanged_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"advance_fraction", "advance_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"decline_fraction", "decline_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"unchanged_fraction", "unchanged_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"directional_participation", "directional_participation", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"breadth", "breadth", string(data.UnitDimensionless), string(data.TimescaleInstantaneous)},
			{"directional_agreement", "directional_agreement", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"directional_consensus", "directional_consensus", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"median_return", "median_return", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"median_absolute_return", "median_absolute_return", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"mean_absolute_return", "mean_absolute_return", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"rms_return", "rms_return", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"return_mad", "return_mad", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"magnitude_mad", "magnitude_mad", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"return_interquartile_range", "return_interquartile_range", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"largest_move_tie_count", "largest_move_tie_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"largest_absolute_return", "largest_absolute_return", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"largest_signed_return", "largest_signed_return", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"largest_move_share", "largest_move_share", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"peer_median_absolute_return", "peer_median_absolute_return", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"peer_magnitude_mad", "peer_magnitude_mad", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"largest_move_excess", "largest_move_excess", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"largest_move_ratio", "largest_move_ratio", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"largest_move_mad_excess", "largest_move_mad_excess", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"same_direction_peer_count", "same_direction_peer_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"opposite_direction_peer_count", "opposite_direction_peer_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"zero_return_peer_count", "zero_return_peer_count", string(data.UnitCount), string(data.TimescaleInstantaneous)},
			{"same_direction_peer_fraction", "same_direction_peer_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"opposite_direction_peer_fraction", "opposite_direction_peer_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"zero_return_peer_fraction", "zero_return_peer_fraction", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"median_asof_age_seconds", "median_asof_age_seconds", string(data.UnitSecond), string(data.TimescaleInstantaneous)},
			{"max_asof_age_seconds", "max_asof_age_seconds", string(data.UnitSecond), string(data.TimescaleInstantaneous)},
			{"median_from_age_seconds", "median_from_age_seconds", string(data.UnitSecond), string(data.TimescaleInstantaneous)},
			{"median_return_baseline", "median_return_baseline", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"median_return_divergence", "median_return_divergence", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"median_return_zscore", "median_return_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"breadth_baseline", "breadth_baseline", string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			{"breadth_divergence", "breadth_divergence", string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			{"breadth_zscore", "breadth_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"median_absolute_return_baseline", "median_absolute_return_baseline", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"median_absolute_return_ratio", "median_absolute_return_ratio", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"median_absolute_return_zscore", "median_absolute_return_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"return_dispersion_baseline", "return_dispersion_baseline", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"return_dispersion_ratio", "return_dispersion_ratio", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"return_dispersion_zscore", "return_dispersion_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"largest_move_share_baseline", "largest_move_share_baseline", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"largest_move_share_zscore", "largest_move_share_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"median_return_velocity", "median_return_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"breadth_velocity", "breadth_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"median_absolute_return_velocity", "median_absolute_return_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"return_dispersion_velocity", "return_dispersion_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"historical_path_distance", "historical_path_distance", string(data.UnitDistance), string(data.TimescaleRollingWindow)},
			{"historical_path_percentile", "historical_path_percentile", string(data.UnitPercent), string(data.TimescaleRollingWindow)},
		},
	}

	signal.System = runtime.NewSystem(ctx, "sentiment", signal)
	return signal
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

	if valid := signal.output.Values["valid_member_count"]; valid > 0 {
		signal.output.Values["cohort_member_count"] = valid
		signal.output.Values["directional_participation"] = (signal.output.Values["advance_count"] + signal.output.Values["decline_count"]) / valid
	}

	out := data.NewMeasurement(
		prior.Epoch, prior.Label, signal.Name(), prior.SeqIdx, prior.Tick,
	)
	out.Peers(prior)
	out.At = prior.At
	out.From = prior.At
	metrics := make([]*data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value := signal.output.Values[metric[1]]

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
