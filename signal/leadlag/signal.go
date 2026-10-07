package leadlag

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

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
	paths     core.Primitive
	pipelines sync.Map
	metrics   map[string][2]string
}

type symbolPipeline struct {
	output   data.Map[float64]
	envelope data.Map[float64]
	state    *data.State
	pipeline core.Primitive
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		paths: store.NewKV[string, [][2]float64](nil),
		// {output key without "@reference"}: {unit, timescale}
		metrics: map[string][2]string{
			"last":                          {string(data.UnitPrice), string(data.TimescaleTick)},
			"observation_count":             {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"pair_observation_count":        {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"best_lag_correlation":          {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"best_lag_covariance":           {string(data.UnitCovariance), string(data.TimescaleRollingWindow)},
			"overlap_pair_count":            {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"return_energy:reference":       {string(data.UnitVariance), string(data.TimescaleRollingWindow)},
			"return_energy:measured":        {string(data.UnitVariance), string(data.TimescaleRollingWindow)},
			"reference_return_count":        {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"measured_return_count":         {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"best_lag_index":                {string(data.UnitIndexOffset), string(data.TimescaleRollingWindow)},
			"best_lag_seconds":              {string(data.UnitTimeOffset), string(data.TimescaleRollingWindow)},
			"leads":                         {string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			"contemporaneous_correlation":   {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"search_count":                  {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"lag_search_resolution_seconds": {string(data.UnitSecond), string(data.TimescaleRollingWindow)},
			"lag_search_span":               {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"lag_search_scale":              {string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			"absolute_correlation_gain":     {string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			"lag_fraction":                  {string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			"lag_peak_prominence":           {string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			"lag_peak_curvature":            {string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			"effective_sample_count":        {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"correlation_p_value":           {string(data.UnitProbability), string(data.TimescaleRollingWindow)},
			"search_adjusted_p_value":       {string(data.UnitProbability), string(data.TimescaleRollingWindow)},
			"lag_baseline_seconds":          {string(data.UnitTimeOffset), string(data.TimescaleRollingWindow)},
			"lag_divergence_seconds":        {string(data.UnitTimeOffset), string(data.TimescaleRollingWindow)},
			"lag_noise_scale_seconds":       {string(data.UnitSecond), string(data.TimescaleRollingWindow)},
			"lag_zscore":                    {string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			"best_lag_correlation_baseline": {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"best_lag_correlation_zscore":   {string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			"correlation_gain_baseline":     {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"correlation_gain_zscore":       {string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			"lag_velocity":                  {string(data.UnitVelocity), string(data.TimescalePerSecond)},
			"correlation_gain_velocity":     {string(data.UnitVelocity), string(data.TimescalePerSecond)},
			"historical_path_distance":      {string(data.UnitDistance), string(data.TimescaleRollingWindow)},
			"historical_path_percentile":    {string(data.UnitPercent), string(data.TimescaleRollingWindow)},
		},
	}

	signal.System = runtime.NewSystem(ctx, "leadlag", signal)
	return signal
}

func (signal *Signal) pipelineFor(symbol string) *symbolPipeline {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(*symbolPipeline)
	}

	output := data.NewOutputMap()

	pipe := &symbolPipeline{
		output:   output,
		envelope: data.NewOutputMap(),
		state:    data.NewState(data.NewMap(), output),
		pipeline: transport.NewStages(
			nmcorrelation.NewMember(symbol, signal.paths, adaptive.NewWindow()),
			nmcorrelation.NewLeads(symbol, signal.paths, algo.NewHayashiYoshida()),
		),
	}

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipe)
	return actual.(*symbolPipeline)
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

	price, err := tradeValue(prior, "price")

	if err != nil {
		signal.Error(err)
		return nil
	}

	if price <= 0 || math.IsInf(price, 0) || math.IsNaN(price) {
		return nil
	}

	pipe := signal.pipelineFor(prior.Label)

	clear(pipe.output.Values)
	clear(pipe.envelope.Values)

	pipe.envelope.Values["at"] = float64(prior.At.UnixNano())
	pipe.envelope.Values["price"] = price
	pipe.envelope.Values["last"] = price

	adapter := data.NewAdapter(prior, pipe.state)

	for range adapter.Next(data.NewValue(pipe.envelope)) {
	}

	for range pipe.pipeline.Next(data.NewValue(adapter)) {
	}

	if err := errors.Join(adapter.Error(), pipe.pipeline.Error()); err != nil {
		signal.Error(err)
		return nil
	}

	out := data.NewMeasurement(
		prior.Epoch, prior.Label, signal.Name(), prior.SeqIdx, prior.Tick,
	)
	out.Peers(prior)
	out.At = prior.At
	out.From = prior.At

	if from, held := pipe.output.Values["path_from"]; held && from <= float64(prior.At.UnixNano()) {
		out.From = time.Unix(0, int64(from)).UTC()
	}

	keys := make([]string, 0, len(pipe.output.Values))

	for key := range pipe.output.Values {
		keys = append(keys, key)
	}

	slices.Sort(keys)
	metrics := make([]*data.Metric, 0, len(keys))

	for _, key := range keys {
		fact, _, _ := strings.Cut(key, "@")
		declared, published := signal.metrics[fact]
		value := pipe.output.Values[key]

		if !published {
			continue
		}

		metrics = append(metrics, data.NewMetric(
			key, value, data.Unit(declared[0]), data.Timescale(declared[1]),
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
			errnie.NotAcceptable, "[leadlag] trade frame is missing "+key, nil,
		)
	}

	return entry.Metric.Raw, nil
}
