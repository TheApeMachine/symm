package correlation

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
Signal is the asynchronous price-path correlation instrument. It holds no
logic of its own: its entire behavior is one nomagique Stages pipeline per
symbol over a shared output map. Member admits the arrival into the symbol's
retained path and publishes that path into the signal's shared path store;
Pairs measures the symbol against every peer path in the store and the cohort
across them. Peers never live on the Measurement: the keyed path store is the
only place symbols meet. Pair facts are published as "<fact>@<reference>",
cohort facts unsuffixed. Facts accumulate in the output map and are written
once.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
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

func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	signal := &Signal{
		arena: arena,
		paths: store.NewKV[string, [][2]float64](nil),
		// {output key without "@reference"}: {unit, timescale}
		metrics: map[string][2]string{
			"last_price":                        {string(data.UnitPrice), string(data.TimescaleTick)},
			"observation_count":                 {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"signed_correlation":                {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"absolute_correlation":              {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"covariance":                        {string(data.UnitVariance), string(data.TimescaleRollingWindow)},
			"overlap_pair_count":                {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"return_energy:reference":           {string(data.UnitVariance), string(data.TimescaleRollingWindow)},
			"return_energy:measured":            {string(data.UnitVariance), string(data.TimescaleRollingWindow)},
			"return_count:reference":            {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"return_count:measured":             {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"supported_return_count:reference":  {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"supported_return_count:measured":   {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"return_energy_rate:reference":      {string(data.UnitRate), string(data.TimescalePerSecond)},
			"return_energy_rate:measured":       {string(data.UnitRate), string(data.TimescalePerSecond)},
			"shared_time":                       {string(data.UnitSecond), string(data.TimescaleRollingWindow)},
			"overlap_density":                   {string(data.UnitPerSecond), string(data.TimescaleRollingWindow)},
			"relative_return_energy":            {string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			"effective_sample_count":            {string(data.UnitCount), string(data.TimescaleRollingWindow)},
			"correlation_baseline":              {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"correlation_divergence":            {string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			"correlation_zscore":                {string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			"relative_return_energy_baseline":   {string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			"relative_return_energy_divergence": {string(data.UnitLogReturn), string(data.TimescaleRollingWindow)},
			"relative_return_energy_zscore":     {string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			"correlation_velocity":              {string(data.UnitVelocity), string(data.TimescalePerSecond)},
			"relative_return_energy_velocity":   {string(data.UnitVelocity), string(data.TimescalePerSecond)},
			"historical_path_distance":          {string(data.UnitDistance), string(data.TimescaleRollingWindow)},
			"historical_path_percentile":        {string(data.UnitPercent), string(data.TimescaleRollingWindow)},
			"cohort_peer_count":                 {string(data.UnitCount), string(data.TimescaleInstantaneous)},
			"cohort_effective_peer_count":       {string(data.UnitCount), string(data.TimescaleInstantaneous)},
			"cohort_signed_correlation":         {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"cohort_absolute_correlation":       {string(data.UnitCorrelation), string(data.TimescaleRollingWindow)},
			"cohort_correlation_dispersion":     {string(data.UnitDimensionless), string(data.TimescaleRollingWindow)},
			"focal_return_energy_rate":          {string(data.UnitRate), string(data.TimescalePerSecond)},
			"peer_return_energy_rate":           {string(data.UnitRate), string(data.TimescalePerSecond)},
			"relative_cohort_return_energy":     {string(data.UnitRatio), string(data.TimescaleRollingWindow)},
		},
	}

	signal.System = runtime.NewSystem(ctx, "correlation", signal)
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

	pipe := &symbolPipeline{
		output:   output,
		envelope: data.NewOutputMap(),
		state:    data.NewState(data.NewMap(), output),
		pipeline: transport.NewStages(
			nmcorrelation.NewMember(symbol, signal.paths, adaptive.NewWindow()),
			nmcorrelation.NewPairs(symbol, signal.paths, algo.NewHayashiYoshida()),
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
	pipe.envelope.Values["last_price"] = price

	adapter := data.NewAdapter(prior, pipe.state)

	for range adapter.Next(data.NewValue(pipe.envelope)) {
	}

	for range pipe.pipeline.Next(data.NewValue(adapter)) {
	}

	if err := errors.Join(
		adapter.Error(), pipe.pipeline.Error(),
	); err != nil {
		signal.Error(err)
		return nil
	}

	var metadata []*data.StringEntry

	if channel := prior.Meta("channel"); channel != "" {
		metadata = append(metadata, &data.StringEntry{
			Key:   "channel",
			Value: channel,
		})
	}

	out := signal.arena.NewMeasurement(
		prior.Epoch,
		prior.Label,
		signal.Name(),
		prior.SeqIdx,
		prior.Tick,
		[]*data.Measurement{prior},
		metadata...,
	)

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
			errnie.NotAcceptable, "[correlation] trade frame is missing "+key, nil,
		)
	}

	return entry.Metric.Raw, nil
}
