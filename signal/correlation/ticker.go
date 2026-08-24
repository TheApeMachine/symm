package correlation

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
Cohort output slots produced by the finalize scoring stage. The raw Cohort
sufficient statistics are reduced into the README's non-interpretive cohort
metrics: signed/absolute correlation, peer energy, and relative return energy.
*/
var (
	symbolCohortReady    = nmtypes.MustIntern("correlation/cohort_ready")
	symbolCohortSigned   = nmtypes.MustIntern("correlation/cohort_signed_correlation")
	symbolCohortAbsolute = nmtypes.MustIntern("correlation/cohort_absolute_correlation")
	symbolPeerEnergy     = nmtypes.MustIntern("correlation/peer_return_energy")
	symbolRelativeEnergy = nmtypes.MustIntern("correlation/relative_return_energy")
)

/*
Pair-history series prefixes. Each holds one focal-level pair metric across
ticks so the namespaced Baseline/ZScore/Velocity estimators can derive its
causal history without touching the per-symbol price path.
*/
const (
	prefixSignedCorrelation = "pair/signed_correlation"
	prefixRelativeEnergy    = "pair/relative_energy"
)

var (
	signedCorrelationSeries = temporal.NewSeries(prefixSignedCorrelation)
	relativeEnergySeries    = temporal.NewSeries(prefixRelativeEnergy)

	symbolCorrelationBaseline   = nmtypes.MustIntern(temporal.JoinPrefix(prefixSignedCorrelation, "baseline/value"))
	symbolCorrelationDivergence = nmtypes.MustIntern(temporal.JoinPrefix(prefixSignedCorrelation, "z/residual"))
	symbolCorrelationZScore     = nmtypes.MustIntern(temporal.JoinPrefix(prefixSignedCorrelation, "z/value"))
	symbolCorrelationVelocity   = nmtypes.MustIntern(temporal.JoinPrefix(prefixSignedCorrelation, "velocity/delta"))

	symbolRelativeEnergyBaseline   = nmtypes.MustIntern(temporal.JoinPrefix(prefixRelativeEnergy, "baseline/value"))
	symbolRelativeEnergyDivergence = nmtypes.MustIntern(temporal.JoinPrefix(prefixRelativeEnergy, "z/residual"))
	symbolRelativeEnergyZScore     = nmtypes.MustIntern(temporal.JoinPrefix(prefixRelativeEnergy, "z/value"))
	symbolRelativeEnergyVelocity   = nmtypes.MustIntern(temporal.JoinPrefix(prefixRelativeEnergy, "velocity/delta"))
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
			pairHistoryStage(prefixSignedCorrelation),
			pairHistoryStage(prefixRelativeEnergy),
		)),
		projector: data.NewProjector(
			data.Binding{From: temporal.DefaultSeries.ValueSymbol, Name: "last_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmtypes.SampleCount, Name: "observation_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolCohortSigned, Name: "signed_correlation", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolCohortAbsolute, Name: "absolute_correlation", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolCovariance, Name: "covariance", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolRightVariance, Name: "return_energy:reference", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolLeftVariance, Name: "return_energy:measured", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolSupport, Name: "overlap_pair_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolSupport, Name: "effective_sample_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolPValue, Name: "correlation_p_value", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolFisherStandardErr, Name: "correlation_standard_error_fisher", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolPeerCount, Name: "cohort_peer_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolCohortDispersion, Name: "cohort_correlation_dispersion", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmcorrelation.SymbolEffectivePeers, Name: "cohort_effective_peer_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeEnergy, Name: "relative_return_energy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolCorrelationBaseline, Name: "correlation_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolCorrelationDivergence, Name: "correlation_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolCorrelationZScore, Name: "correlation_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolCorrelationVelocity, Name: "correlation_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeEnergyBaseline, Name: "relative_return_energy_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeEnergyDivergence, Name: "relative_return_energy_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeEnergyZScore, Name: "relative_return_energy_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeEnergyVelocity, Name: "relative_return_energy_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
		),
		pair:     newCorrelationPair(),
		reduce:   nmcorrelation.Cohort,
		finalize: cohortFinalize(),
	}
}

/*
Step receives one market data point, appends the price into the symbol's
retained path, evaluates the cross-sectional cohort over every other committed
path, advances the focal-level pair-history estimators, and projects exactly one
Measurement. The per-symbol path facts are always projected; cohort and
pair-history facts appear only once their estimators are ready.
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
		if ready, found := crossSectionFrame.Get(symbolCohortReady); found && ready != 0 {
			projection.Merge(crossSectionFrame)
			projection.Merge(ticker.stepPairHistory(tickerData.Symbol, tickerData.Timestamp, crossSectionFrame))
		}
	}

	return ticker.projector.Project(
		ticker.label(tickerData.Symbol),
		"correlation",
		tickerData.Timestamp,
		tickerData.Timestamp,
		projection,
	)
}

func (ticker *Ticker) Close() error { return nil }

/*
stepPairHistory feeds the focal-level cohort metrics into their namespaced
pair-history series and evaluates the causal Baseline/ZScore/Velocity estimators.
It runs only once the cohort is ready, so every required coordinate is present.
*/
func (ticker *Ticker) stepPairHistory(
	symbol string,
	at time.Time,
	crossSection nmtypes.Frame,
) nmtypes.Frame {
	signed, hasSigned := crossSection.Get(symbolCohortSigned)
	relative, hasRelative := crossSection.Get(symbolRelativeEnergy)

	if !hasSigned || !hasRelative {
		return nmtypes.Frame{}
	}

	input := nmtypes.Frame{}
	loadPairHistory(&input, signedCorrelationSeries, signed, at)
	loadPairHistory(&input, relativeEnergySeries, relative, at)

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
newCorrelationPair relocates the focal committed path into the "previous"
series and the peer path into the "current" series, evaluates the Hayashi-Yoshida
asynchronous correlation, then derives the Fisher p-value for the estimate.
*/
func newCorrelationPair() func(nmtypes.Frame, nmtypes.Frame) nmtypes.Frame {
	previous := temporal.NewSeries("previous")
	current := temporal.NewSeries("current")
	hayashi := nmcorrelation.Hayashi("previous", "current")

	return func(focal nmtypes.Frame, peer nmtypes.Frame) nmtypes.Frame {
		paired := nmtypes.Frame{}
		previous.CopyFrom(&paired, focal)
		current.CopyFrom(&paired, peer)

		return nmcorrelation.PValue(hayashi(paired))
	}
}

/*
cohortFinalize is the declarative scoring stage that reduces Cohort's sufficient
statistics into the README's non-interpretive cohort metrics. It is composed
only from nomagique primitives; readiness gates the reduction so an unready
cohort emits no metric slots instead of an error.
*/
func cohortFinalize() nmtypes.Primitive {
	predicate := nmtypes.Relay(nmcorrelation.SymbolReady, logic.SymbolCondition)

	whenTrue := nmtypes.Pipe(
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(nmcorrelation.SymbolWeightedSigned, calculus.PortA),
			nmtypes.In(nmcorrelation.SymbolTotalSupport, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolCohortSigned),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(nmcorrelation.SymbolWeightedAbsolute, calculus.PortA),
			nmtypes.In(nmcorrelation.SymbolTotalSupport, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolCohortAbsolute),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(nmcorrelation.SymbolWeightedPeerEnergy, calculus.PortA),
			nmtypes.In(nmcorrelation.SymbolTotalSupport, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolPeerEnergy),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(nmcorrelation.SymbolFocalEnergy, calculus.PortA),
			nmtypes.In(symbolPeerEnergy, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolRelativeEnergy),
		),
		nmtypes.Assign(symbolCohortReady, 1),
	)

	whenFalse := nmtypes.Assign(symbolCohortReady, 0)

	return logic.If(predicate, whenTrue, whenFalse)
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
