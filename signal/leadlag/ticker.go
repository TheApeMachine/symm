package leadlag

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Ticker is the per-symbol price-path entity.

It connects market data to one nomagique pipeline per focal symbol and names
the readings that pipeline produces. It holds no mathematics and no
estimator state of its own: the paths, the lag search, the significance test
and the causal estimators are all nodes inside the composition.
*/
type Ticker struct {
	mutex sync.Mutex

	// paths is the retained observation store per symbol. The lag search is
	// cross-sectional, so a focal symbol is measured against every other path.
	paths map[string]*nmcorrelation.Path

	// pipelines holds one composed pipeline per focal symbol, so each symbol's
	// estimators accumulate their own history.
	pipelines map[string]*pipeline
}

/*
pipeline is one focal symbol's complete composition, declared once in
newPipeline and stepped once per tick.
*/
type pipeline struct {
	leadLag nmcorrelation.LeadLag
	fisher  nmcorrelation.Fisher

	// Causal estimators over this signal's focal-level readings, each fed by
	// a Tap on the search stage.
	lagSeconds         equation.CausalResidual
	gain               equation.CausalResidual
	bestLagCorrelation equation.CausalResidual

	lagVelocity  temporal.Velocity
	gainVelocity temporal.Velocity
	eventClock   temporal.Clock

	number *nomagique.Pipeline
}

/*
newPipeline declares one focal symbol's whole measurement as a single
composition. It is the signal's entire mathematics.

The Chain runs three stages in series:
  - A searches the lead-lag surface across every emergent shift.
  - B tests the peak's significance, reading its own support and search
    breadth back out of the search stage.
  - C fans out to the causal estimators over the readings that search
    exposes. Each branch is a Tap, which returns 0, so the carrier passes
    through the Split uncorrupted (Law of Sinks).
*/
func newPipeline(focal *nmcorrelation.Path, peer *nmcorrelation.Path) *pipeline {
	built := &pipeline{}

	search := &built.leadLag
	search.Left = focal
	search.Right = peer

	built.fisher.Support = &nomagique.Tap{Read: search.Support}
	built.fisher.SearchCount = &nomagique.Tap{Read: search.SearchCount}

	built.lagVelocity.Source = &nomagique.Tap{Read: search.LagSeconds}
	built.lagVelocity.Clock = &built.eventClock
	built.gainVelocity.Source = &nomagique.Tap{Read: search.AbsoluteGain}
	built.gainVelocity.Clock = &built.eventClock

	built.number = nomagique.Number(&nomagique.Chain{
		A: search,
		B: &built.fisher,
		C: &nomagique.Split{
			A: &nomagique.Split{
				A: &nomagique.Tap{Read: search.LagSeconds, Into: &built.lagSeconds},
				B: &nomagique.Tap{Read: search.AbsoluteGain, Into: &built.gain},
				C: &nomagique.Tap{
					Read: search.LagCorrelation,
					Into: &built.bestLagCorrelation,
				},
			},
			B: &nomagique.Split{
				A: &built.lagVelocity,
				B: &built.gainVelocity,
			},
		},
	})

	return built
}

/*
NewTicker constructs the Ticker entity.
*/
func NewTicker() *Ticker {
	return &Ticker{
		paths:     make(map[string]*nmcorrelation.Path),
		pipelines: make(map[string]*pipeline),
	}
}

/*
Step receives one market data point, records it into the symbol's retained
path, steps that symbol's pipeline against every other observed path, and
names exactly one Measurement from the readings the pipeline exposes.

An explicit zero last means no recent trade price was observed: it produces
an undefined, zero-support measurement without entering the path, so an
untraded quote never becomes evidence.
*/
func (ticker *Ticker) Step(tickerData kraken.TickerData) *data.Measurement[float64] {
	if tickerData.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf(
			"leadlag: ticker requires a last price",
		)}
	}

	last := tickerData.Last.Float64()

	if math.IsNaN(last) || math.IsInf(last, 0) || last < 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf(
			"leadlag: ticker last price must be finite and non-negative",
		)}
	}

	measurement := data.NewMeasurement[float64](
		tickerData.Symbol+":leadlag:"+tickerData.Timestamp.Format(time.RFC3339Nano),
		tickerData.Symbol,
		"leadlag",
		tickerData.Timestamp,
		tickerData.Timestamp,
	)

	if last == 0 {
		measurement.Metadata = map[string]float64{data.MetadataSupport: 0}
		measurement.Provenance = map[string]string{
			"last_trade_price_state": "unobserved",
		}
		measurement.Finalize()

		return measurement
	}

	ticker.mutex.Lock()
	defer ticker.mutex.Unlock()

	focal := ticker.pathFor(tickerData.Symbol)

	if !focal.Observe(tickerData.Timestamp.UnixNano(), nmtypes.Number(last)) {
		measurement.Err = fmt.Errorf(
			"leadlag: ticker event time regressed for %s", tickerData.Symbol,
		)

		return measurement
	}

	putMetric(measurement, "last_price", nmtypes.Number(last),
		data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "observation_count", nmtypes.Number(focal.Len()),
		data.UnitCount, data.TimescaleInstantaneous)

	built := ticker.step(tickerData.Symbol, focal, tickerData.Timestamp)

	if built == nil {
		measurement.Metadata = map[string]float64{data.MetadataSupport: 0}
		measurement.Finalize()

		return measurement
	}

	built.project(measurement)

	if from, _, ok := focal.Span(); ok {
		measurement.From = time.Unix(0, from)
	}

	measurement.Finalize()

	return measurement
}

func (ticker *Ticker) pathFor(symbol string) *nmcorrelation.Path {
	path, found := ticker.paths[symbol]

	if !found {
		path = &nmcorrelation.Path{}
		ticker.paths[symbol] = path
	}

	return path
}

/*
step drives the focal symbol's pipeline against each peer path, returning the
pipeline once any peer yielded a defined estimate.
*/
func (ticker *Ticker) step(
	symbol string,
	focal *nmcorrelation.Path,
	at time.Time,
) *pipeline {
	var defined *pipeline

	for peerSymbol, peer := range ticker.paths {
		if peerSymbol == symbol {
			continue
		}

		built := ticker.pipelineFor(symbol, focal, peer)
		built.tick(at)

		if built.leadLag.Ready() {
			defined = built
		}
	}

	return defined
}

func (ticker *Ticker) pipelineFor(
	symbol string,
	focal *nmcorrelation.Path,
	peer *nmcorrelation.Path,
) *pipeline {
	built, found := ticker.pipelines[symbol]

	if !found {
		built = newPipeline(focal, peer)
		ticker.pipelines[symbol] = built
	}

	// The focal symbol is measured against each peer in turn; only the peer
	// slot changes between steps.
	built.leadLag.Right = peer

	return built
}

/*
tick advances the composition's event clock and steps the whole pipeline once.
*/
func (built *pipeline) tick(at time.Time) {
	built.eventClock.Observe(at)
	built.number.Step(0)
}

/*
project names every reading the composition exposes. Each value is read from
a node; the signal derives none of them.
*/
func (built *pipeline) project(measurement *data.Measurement[float64]) {
	search := &built.leadLag

	putMetric(measurement, "contemporaneous_correlation", search.Contemporaneous(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "best_lag_correlation", search.LagCorrelation(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "best_lag_index", search.LagBars(),
		data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "best_lag_seconds", search.LagSeconds(),
		data.UnitSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "absolute_correlation_gain", search.AbsoluteGain(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "lag_fraction", search.LagFraction(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "lag_search_resolution_seconds",
		search.SearchResolutionSeconds(),
		data.UnitSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "lag_search_span",
		search.SearchSpanSeconds(),
		data.UnitSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "reference_return_count", search.LeftReturns(),
		data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "measured_return_count", search.RightReturns(),
		data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "overlap_pair_count", search.Support(),
		data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "effective_sample_count", search.Support(),
		data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "search_count", search.SearchCount(),
		data.UnitCount, data.TimescaleInstantaneous)

	if prominence, ok := search.PeakProminence(); ok {
		putMetric(measurement, "lag_peak_prominence", prominence,
			data.UnitDimensionless, data.TimescaleInstantaneous)
	}

	if curvature, ok := search.PeakCurvature(); ok {
		putMetric(measurement, "lag_peak_curvature", curvature,
			data.UnitPerSecond, data.TimescaleInstantaneous)
	}

	if built.fisher.Ready() {
		putMetric(measurement, "correlation_p_value", built.fisher.PValue(),
			data.UnitDimensionless, data.TimescaleInstantaneous)

		if adjusted, ok := built.fisher.SearchAdjustedPValue(); ok {
			putMetric(measurement, "search_adjusted_p_value", adjusted,
				data.UnitDimensionless, data.TimescaleInstantaneous)
		}
	}

	putMetric(measurement, "lag_baseline_seconds", built.lagSeconds.Baseline(),
		data.UnitSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "lag_divergence_seconds", built.lagSeconds.Residual(),
		data.UnitSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "lag_noise_scale_seconds", built.lagSeconds.Dispersion(),
		data.UnitSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "lag_zscore", built.lagSeconds.ZScore(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "lag_velocity", built.lagVelocity.Rate(),
		data.UnitPerSecond, data.TimescaleInstantaneous)

	putMetric(measurement, "correlation_gain_baseline", built.gain.Baseline(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "correlation_gain_zscore", built.gain.ZScore(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "correlation_gain_velocity", built.gainVelocity.Rate(),
		data.UnitPerSecond, data.TimescaleInstantaneous)

	putMetric(measurement, "best_lag_correlation_baseline",
		built.bestLagCorrelation.Baseline(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "best_lag_correlation_zscore",
		built.bestLagCorrelation.ZScore(),
		data.UnitDimensionless, data.TimescaleInstantaneous)

	// best_lag_correlation is the headline reading, so its estimator supplies
	// the quality facts Finalize derives maturity and SNR from.
	if measurement.Metadata == nil {
		measurement.Metadata = map[string]float64{}
	}

	measurement.Metadata[data.MetadataSupport] = built.bestLagCorrelation.Count()

	if built.bestLagCorrelation.HasPrior() {
		measurement.Metadata[data.MetadataDivergence] =
			float64(built.bestLagCorrelation.Residual())
		measurement.Metadata[data.MetadataNoiseVariance] =
			float64(built.bestLagCorrelation.NoiseVariance())
	}
}

func (ticker *Ticker) Close() error { return nil }

func putMetric(
	measurement *data.Measurement[float64],
	label string,
	raw nmtypes.Number,
	unit data.Unit,
	timescale data.Timescale,
) {
	measurement.PutMetric(data.Metric[float64]{
		Label:     label,
		Raw:       float64(raw),
		Unit:      unit,
		Timescale: timescale,
	})
}
