package leadlag

import (
	"sort"
	"strings"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Finalize output slots derived from CrossLag's raw nanoseconds quantities:
seconds-valued lag metrics and the absolute correlation gain. CrossLag already
emits the signed contemporaneous and best-lag correlations, the lag index, the
lag fraction, the search/sample/overlap/effective-support counts, and the
peak-shape facts; the finalize only rescales and derives.
*/
var (
	symbolAbsoluteCorrelationGain       = nmtypes.MustIntern("leadlag/absolute_correlation_gain")
	symbolBestLagSeconds                = nmtypes.MustIntern("leadlag/best_lag_seconds")
	symbolLagSearchResolutionSec        = nmtypes.MustIntern("leadlag/lag_search_resolution_seconds")
	symbolLagSearchSpanSeconds          = nmtypes.MustIntern("leadlag/lag_search_span")
	symbolNanosPerSecond                = nmtypes.MustIntern("leadlag/nanos_per_second")
	symbolNegBestLagNanos               = nmtypes.MustIntern("leadlag/neg_best_lag_nanos")
	symbolAbsBestLagCorrelation         = nmtypes.MustIntern("leadlag/abs_best_lag_correlation")
	symbolAbsContemporaneousCorrelation = nmtypes.MustIntern("leadlag/abs_contemporaneous_correlation")
)

/*
Pair-history series prefixes. Each holds one focal-level pair metric across
ticks so the namespaced Baseline/ZScore/Velocity estimators can derive its
causal history without touching the per-symbol price path.
*/
const (
	prefixLagSeconds         = "pair/lag_seconds"
	prefixGain               = "pair/gain"
	prefixBestLagCorrelation = "pair/best_lag_correlation"
)

var (
	lagSecondsSeries         = temporal.NewSeries(prefixLagSeconds)
	gainSeries               = temporal.NewSeries(prefixGain)
	bestLagCorrelationSeries = temporal.NewSeries(prefixBestLagCorrelation)

	symbolLagBaselineSeconds   = nmtypes.MustIntern(temporal.JoinPrefix(prefixLagSeconds, "baseline/value"))
	symbolLagDivergenceSeconds = nmtypes.MustIntern(temporal.JoinPrefix(prefixLagSeconds, "z/residual"))
	symbolLagNoiseScaleSeconds = nmtypes.MustIntern(temporal.JoinPrefix(prefixLagSeconds, "z/dispersion"))
	symbolLagZScore            = nmtypes.MustIntern(temporal.JoinPrefix(prefixLagSeconds, "z/value"))
	symbolLagVelocity          = nmtypes.MustIntern(temporal.JoinPrefix(prefixLagSeconds, "velocity/delta"))

	symbolGainBaseline = nmtypes.MustIntern(temporal.JoinPrefix(prefixGain, "baseline/value"))
	symbolGainZScore   = nmtypes.MustIntern(temporal.JoinPrefix(prefixGain, "z/value"))
	symbolGainVelocity = nmtypes.MustIntern(temporal.JoinPrefix(prefixGain, "velocity/delta"))

	symbolBestLagCorrelationBaseline = nmtypes.MustIntern(temporal.JoinPrefix(prefixBestLagCorrelation, "baseline/value"))
	symbolBestLagCorrelationZScore   = nmtypes.MustIntern(temporal.JoinPrefix(prefixBestLagCorrelation, "z/value"))
)

/*
Ticker is the per-symbol price-path entity. It owns the per-symbol path Number,
the pair-history Number for focal-level causal estimators, one projector naming
the output slots, and the once-built cross-sectional stages.
*/
type Ticker struct {
	number      *nomagique.Number[string]
	pairHistory *nomagique.Number[string]
	projector   *data.Projector
	pair        func(nmtypes.Frame, nmtypes.Frame) nmtypes.Frame
	reduce      nmtypes.Primitive
	finalize    nmtypes.Primitive
}

/*
NewTicker constructs the Ticker entity: the per-symbol path pipeline, the
pair-history pipeline, the projector, and the cross-sectional stages built
exactly once.
*/
func NewTicker() *Ticker {
	return &Ticker{
		number: nomagique.NewNumber[string](temporal.Path("")),
		pairHistory: nomagique.NewNumber[string](nmtypes.Fork(
			pairHistoryStage(prefixLagSeconds),
			pairHistoryStage(prefixGain),
			pairHistoryStage(prefixBestLagCorrelation),
		)),
		projector: data.NewProjector(
			data.Binding{From: temporal.DefaultSeries.ValueSymbol, Name: "last_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmtypes.SampleCount, Name: "observation_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolContempCorrelation, Name: "contemporaneous_correlation", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolLagCorrelation, Name: "best_lag_correlation", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolLagBars, Name: "best_lag_index", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBestLagSeconds, Name: "best_lag_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAbsoluteCorrelationGain, Name: "absolute_correlation_gain", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolLagFraction, Name: "lag_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLagSearchResolutionSec, Name: "lag_search_resolution_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLagSearchSpanSeconds, Name: "lag_search_span", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolReferenceReturns, Name: "reference_return_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolMeasuredReturns, Name: "measured_return_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolOverlapPairs, Name: "overlap_pair_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolEffectiveSupport, Name: "effective_sample_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolLeadLagSearchCount, Name: "search_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolLagPeakProminence, Name: "lag_peak_prominence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolLagPeakCurvature, Name: "lag_peak_curvature", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolPValue, Name: "correlation_p_value", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolSearchAdjustedP, Name: "search_adjusted_p_value", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLagBaselineSeconds, Name: "lag_baseline_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLagDivergenceSeconds, Name: "lag_divergence_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLagNoiseScaleSeconds, Name: "lag_noise_scale_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLagZScore, Name: "lag_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLagVelocity, Name: "lag_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolGainBaseline, Name: "correlation_gain_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolGainZScore, Name: "correlation_gain_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolGainVelocity, Name: "correlation_gain_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBestLagCorrelationBaseline, Name: "best_lag_correlation_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBestLagCorrelationZScore, Name: "best_lag_correlation_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
		pair:     newLeadLagPair(),
		reduce:   nmcorrelation.Cohort,
		finalize: leadLagFinalize(),
	}
}

/*
Step receives one market data point, appends the price into the symbol's
retained path, evaluates the cross-sectional lag search over every other
committed path, advances the focal-level pair-history estimators, and projects
exactly one Measurement. The per-symbol path facts are always projected;
lead-lag and pair-history facts appear only once their estimators are ready.
*/
func (ticker *Ticker) Step(tickerData kraken.TickerData) *data.Measurement[float64] {
	input := nmtypes.Frame{}
	input.Put(temporal.DefaultSeries.ValueSymbol, tickerData.Last.Float64())
	input.Put(temporal.DefaultSeries.SecSymbol, float64(tickerData.Timestamp.Unix()))
	input.Put(temporal.DefaultSeries.NsecSymbol, float64(tickerData.Timestamp.Nanosecond()))

	pathFrame := ticker.number.Step(tickerData.Symbol, input)

	crossSectionFrame, reduced, err := ticker.number.CrossSection(
		tickerData.Symbol,
		ticker.pair,
		ticker.reduce,
		ticker.finalize,
	)

	projection := pathFrame

	if err == nil && reduced {
		if ready, found := crossSectionFrame.Get(nmcorrelation.SymbolLeadLagReady); found && ready != 0 {
			projection.Merge(crossSectionFrame)
			projection.Merge(ticker.stepPairHistory(tickerData.Symbol, tickerData.Timestamp, crossSectionFrame))
		}
	}

	return ticker.projector.Project(
		ticker.label(tickerData.Symbol),
		"leadlag",
		tickerData.Timestamp,
		tickerData.Timestamp,
		projection,
	)
}

func (ticker *Ticker) Close() error { return nil }

/*
stepPairHistory feeds the focal-level lag metrics into their namespaced
pair-history series and evaluates the causal Baseline/ZScore/Velocity estimators.
It runs only once CrossLag is ready, so every required coordinate is present.
*/
func (ticker *Ticker) stepPairHistory(
	symbol string,
	at time.Time,
	crossSection nmtypes.Frame,
) nmtypes.Frame {
	lagSeconds, hasLagSeconds := crossSection.Get(symbolBestLagSeconds)
	gain, hasGain := crossSection.Get(symbolAbsoluteCorrelationGain)
	bestLagCorrelation, hasBestLagCorrelation := crossSection.Get(nmcorrelation.SymbolLagCorrelation)

	if !hasLagSeconds || !hasGain || !hasBestLagCorrelation {
		return nmtypes.Frame{}
	}

	input := nmtypes.Frame{}
	loadPairHistory(&input, lagSecondsSeries, lagSeconds, at)
	loadPairHistory(&input, gainSeries, gain, at)
	loadPairHistory(&input, bestLagCorrelationSeries, bestLagCorrelation, at)

	return ticker.pairHistory.Step(symbol, input)
}

func loadPairHistory(
	input *nmtypes.Frame,
	series temporal.Series,
	value float64,
	at time.Time,
) {
	input.Put(series.ValueSymbol, value)
	input.Put(series.SecSymbol, float64(at.Unix()))
	input.Put(series.NsecSymbol, float64(at.Nanosecond()))
}

/*
label names the measurement with its measured symbol and, when a peer exists,
the explicit reference symbol(s) separated by a colon. The reference identity is
string provenance that float metrics cannot carry, so it rides the Label.
*/
func (ticker *Ticker) label(symbol string) string {
	peers := make([]string, 0)

	ticker.number.Range(func(key string, _ nmtypes.Frame) bool {
		if key != symbol {
			peers = append(peers, key)
		}

		return true
	})

	if len(peers) == 0 {
		return symbol
	}

	sort.Strings(peers)

	return symbol + ":" + strings.Join(peers, ",")
}

/*
newLeadLagPair relocates the focal committed path into the "previous" series and
the peer path into the "current" series, scans the cross-lag correlation surface,
then derives the Fisher p-values for the selected best lag.
*/
func newLeadLagPair() func(nmtypes.Frame, nmtypes.Frame) nmtypes.Frame {
	previous := temporal.NewSeries("previous")
	current := temporal.NewSeries("current")
	crossLag := nmcorrelation.CrossLag("previous", "current")

	return func(focal nmtypes.Frame, peer nmtypes.Frame) nmtypes.Frame {
		paired := nmtypes.Frame{}
		previous.CopyFrom(&paired, focal)
		current.CopyFrom(&paired, peer)

		output := crossLag(paired)

		if ready, found := output.Get(nmcorrelation.SymbolLeadLagReady); !found || ready == 0 {
			return output
		}

		// PValue reads the generic correlation/support/search-count slots; relocate
		// CrossLag's best-lag facts into those coordinates before deriving p-values.
		output.Put(nmcorrelation.SymbolCorrelation, output.MustGet(nmcorrelation.SymbolLagCorrelation))
		output.Put(nmcorrelation.SymbolSupport, output.MustGet(nmcorrelation.SymbolEffectiveSupport))
		output.Put(nmcorrelation.SymbolSearchCount, output.MustGet(nmcorrelation.SymbolLeadLagSearchCount))

		return nmcorrelation.PValue(output)
	}
}

/*
leadLagFinalize is the declarative scoring stage rescaling CrossLag's nanosecond
lag quantities into seconds and deriving the absolute correlation gain
(|best lag correlation| - |contemporaneous correlation|). Readiness gates the
derivation so an unready search emits no derived slot instead of an error.
*/
func leadLagFinalize() nmtypes.Primitive {
	predicate := nmtypes.Relay(nmcorrelation.SymbolLeadLagReady, logic.SymbolCondition)

	whenTrue := nmtypes.Pipe(
		nmtypes.Assign(symbolNanosPerSecond, 1e9),
		nmtypes.Wire(
			calculus.Negative,
			nmtypes.In(nmcorrelation.SymbolBestLagNanos, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolNegBestLagNanos),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(symbolNegBestLagNanos, calculus.PortA),
			nmtypes.In(symbolNanosPerSecond, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolBestLagSeconds),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(nmcorrelation.SymbolLagSearchResolution, calculus.PortA),
			nmtypes.In(symbolNanosPerSecond, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolLagSearchResolutionSec),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(nmcorrelation.SymbolLagSearchSpan, calculus.PortA),
			nmtypes.In(symbolNanosPerSecond, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolLagSearchSpanSeconds),
		),
		nmtypes.Wire(
			calculus.Absolute,
			nmtypes.In(nmcorrelation.SymbolLagCorrelation, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolAbsBestLagCorrelation),
		),
		nmtypes.Wire(
			calculus.Absolute,
			nmtypes.In(nmcorrelation.SymbolContempCorrelation, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolAbsContemporaneousCorrelation),
		),
		nmtypes.Wire(
			calculus.Difference,
			nmtypes.In(symbolAbsBestLagCorrelation, calculus.PortA),
			nmtypes.In(symbolAbsContemporaneousCorrelation, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolAbsoluteCorrelationGain),
		),
	)

	return logic.If(predicate, whenTrue, nil)
}

/*
pairHistoryStage is the causal estimator chain for one namespaced pair-metric
series: retain the value, evaluate it against the previous baseline, then update
the baseline, then difference for velocity.
*/
func pairHistoryStage(prefix string) nmtypes.Primitive {
	return nmtypes.Pipe(
		temporal.Window(prefix),
		statistic.ZScore(prefix),
		statistic.Baseline(prefix),
		statistic.Velocity(prefix),
	)
}
