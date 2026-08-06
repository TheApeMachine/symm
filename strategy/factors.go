package strategy

import (
	"math"
	"strings"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

/*
causalRowWidth is how many columns a causal history row must carry to be usable
by the trajectory search.
*/
const causalRowWidth = 4

/*
graphContradictionShare is the fraction of a symbol's weighted relational
evidence that must oppose the trade before the hypothesis is abandoned.
*/
const graphContradictionShare = 0.5

/*
unifiedUtility synthesizes forecast edge, Bayesian precision, and corroboration into one decision metric.
*/
func unifiedUtility(
	executableFraction, causal, cognition, graphVal, uncertainty float64,
) float64 {
	if math.IsNaN(executableFraction) || math.IsInf(executableFraction, 0) || executableFraction == 0 {
		return 0
	}

	utility := executableFraction * uncertaintyWeight(uncertainty) *
		causal * cognition * graphVal

	if math.IsNaN(utility) || math.IsInf(utility, 0) {
		return 0
	}

	return utility
}

/*
uncertaintyWeight is the precision factor applied to executable return.
*/
func uncertaintyWeight(uncertainty float64) float64 {
	if math.IsNaN(uncertainty) || math.IsInf(uncertainty, 0) || uncertainty <= 0 {
		return 1
	}

	return 1.0 / (1.0 + uncertainty)
}

/*
causalFactor reads the causal head as corroboration of the forecast, on (0, 2).
*/
func causalFactor(doExpectation, uplift, noise float64, ready bool) float64 {
	if !ready {
		return 1
	}

	score := doExpectation + uplift - noise

	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 1
	}

	return 1 + math.Tanh(score)
}

/*
cognitionFactor scales the forecast by attractor basin confidence and branch entropy, on (0, 1].
*/
func cognitionFactor(cognition types.Cognition) float64 {
	if !cognition.Ready || cognition.LookaheadPaths <= 0 {
		return 1
	}

	if cognition.Confidence > 0 && !math.IsNaN(cognition.Confidence) && !math.IsInf(cognition.Confidence, 0) {
		return math.Max(0.1, math.Min(1.0, cognition.Confidence))
	}

	return 1
}

/*
graphFactor reads the relational graph as consistency with everything else the market is saying, on [0, 1].
*/
func graphFactor(supports, contradicts float64, hasGraph bool) float64 {
	total := supports + contradicts

	if !hasGraph || contradicts <= 0 || total <= 0 {
		return 1
	}

	return 1 - math.Min(1, contradicts/total)
}

/*
adverseSelection prices the risk of being filled by someone better informed.
*/
func adverseSelection(thesis *types.Thesis, row candidate) *decimal.Decimal {
	informed := 0.0

	if thesis != nil && thesis.Causal != nil {
		if raw, found := thesis.Causal.Load(row.Symbol); found {
			if metrics, ok := raw.(map[string]any); ok {
				informed = getFloat(metrics, "informedFlow")
			}
		}
	}

	if math.IsNaN(informed) || math.IsInf(informed, 0) || informed <= 0 {
		return decimal.NewFromInt64(0)
	}

	return row.ExpectedSpread.SetScale(8).Mul(
		decimal.NewFromFloat64(math.Min(1, informed)),
	)
}

/*
exhaustionHoldDiscount converts the exhaustion equation's fused long-side
urgency into the survival discount used by the trajectory search. The equation
already combines mechanical, thermal, fragile, and reversal evidence; strategy
preserves that result rather than imposing a second fusion rule.
*/
func exhaustionHoldDiscount(thesis *types.Thesis, symbol string) (float64, bool) {
	measurement, ok := latestMeasurement(thesis, symbol, types.SourceExhaustion)

	if !ok {
		return 0, false
	}

	urgency, ok := rawMetric(measurement, types.MetricUrgency, types.SideBuy)

	if !ok || urgency < 0 || urgency > 1 {
		return 0, false
	}

	discount := math.Exp(-urgency)

	if discount >= 1 {
		// A clean reading may approach one but MCTS gamma remains convergent.
		discount = math.Nextafter(1, 0)
	}

	return discount, discount > 0 && discount < 1
}

/*
hawkesSpectralRadius reads the fitted branching matrix's spectral radius for
the symbol. The Hawkes estimator is stationary by contract, so one is excluded:
a value at or above it is not a stronger reading from this model but a fit that
violates the model whose output strategy is consuming.
*/
func hawkesSpectralRadius(thesis *types.Thesis, symbol string) (float64, bool) {
	measurement, ok := latestMeasurement(thesis, symbol, types.SourceHawkes)

	if !ok {
		return 0, false
	}

	spectralRadius, ok := rawMetric(
		measurement, types.MetricSpectralRadius, types.SideNone,
	)

	if !ok || spectralRadius < 0 || spectralRadius >= 1 {
		return 0, false
	}

	return spectralRadius, true
}

/*
ignitionPrecursor answers whether this symbol is winding up a long-side
ignition, using the same model-owned baselines regimeExit reads for the short
side.

Presence of a category is not the question. The category solver names a vertical
ignition wherever the shape is present at all, so a quiet market carries the name
with a strength near its own noise while a real one carries it an order of
magnitude higher, and a rule keyed on the name alone admits both. Measured across
one pumping symbol and two baseline ones, ignition read 7.25 against 0.65 and
0.00, and compression 0.49 against 0.00 and 0.00, while every one of the three
was labelled a vertical ignition.

So it is the readings that decide. Ignition is normalised against the symbol's
own empirical baseline, which makes the unit a measured statement that this
market is doing something it does not ordinarily do rather than a threshold
anybody chose, and it has to dominate the opposing side to be directional at all.
Compression is the coil that separates a wind-up from a move already spent.
*/
func ignitionPrecursor(thesis *types.Thesis, symbol string) bool {
	pump, ok := latestMeasurement(thesis, symbol, types.SourcePumpDump)

	if !ok {
		return false
	}

	buy, buyReady := rawMetric(pump, types.MetricIgnition, types.SideBuy)
	sell, sellReady := rawMetric(pump, types.MetricIgnition, types.SideSell)

	if !buyReady || !sellReady || buy <= 1 || buy <= sell {
		return false
	}

	compression, compressionReady := rawMetric(
		pump, types.MetricCompression, types.SideBuy,
	)

	return compressionReady && compression > 0
}

/*
regimeExit names a structural long-position invalidation using the signal
families that already model directional ignition and self-exciting cascades.
Both comparisons use model-owned baselines: ignition must exceed its empirical
unit baseline, while a sell parent must be expected to produce at least one
descendant and dominate the buy process.
*/
func regimeExit(thesis *types.Thesis, symbol string) string {
	pump, ok := latestMeasurement(thesis, symbol, types.SourcePumpDump)

	if ok {
		sell, sellReady := rawMetric(pump, types.MetricIgnition, types.SideSell)
		buy, buyReady := rawMetric(pump, types.MetricIgnition, types.SideBuy)

		if sellReady && buyReady && sell > 1 && sell > buy {
			return types.TriggerPumpDumpSellIgnition
		}
	}

	hawkes, ok := latestMeasurement(thesis, symbol, types.SourceHawkes)

	if !ok {
		return ""
	}

	spectral, spectralReady := rawMetric(
		hawkes, types.MetricSpectralRadius, types.SideNone,
	)

	sellDescendants, sellDescendantsReady := rawMetric(
		hawkes, types.MetricTotalDescendants, types.SideSell,
	)

	buyDescendants, buyDescendantsReady := rawMetric(
		hawkes, types.MetricTotalDescendants, types.SideBuy,
	)

	sellIntensity, sellIntensityReady := rawMetric(
		hawkes, types.MetricConditionalIntensity, types.SideSell,
	)

	buyIntensity, buyIntensityReady := rawMetric(
		hawkes, types.MetricConditionalIntensity, types.SideBuy,
	)

	if spectralReady && spectral > 0 &&
		sellDescendantsReady && buyDescendantsReady &&
		sellIntensityReady && buyIntensityReady &&
		sellDescendants >= 1 && sellDescendants > buyDescendants &&
		sellIntensity > buyIntensity {
		return types.TriggerHawkesSellCascade
	}

	return ""
}

/*
allocationHaircut gates overlapping microstructure warnings through their
largest penalty odds. Scarcity, hollowing, and adverse selection can all be
different observations of one withdrawal of executable liquidity; adding their
odds would count that one cause repeatedly. The reason retains every live input
so the overlap gate remains observable even though only its maximum reaches
the allocator.
*/
func allocationHaircut(
	thesis *types.Thesis,
	row candidate,
	adverse *decimal.Decimal,
) (float64, string, bool) {
	liquidity, ok := latestMeasurement(thesis, row.Symbol, types.SourceLiquidity)

	if !ok {
		return 0, "", false
	}

	scarcity, ok := rawMetric(
		liquidity, types.MetricScarcityScore, types.SideNone,
	)

	if !ok || scarcity < 0 || scarcity > 1 {
		return 0, "", false
	}

	hollow, hollowReady := hollowPressure(thesis, row.Symbol)

	if !hollowReady || hollow < 0 || hollow > 1 {
		return 0, "", false
	}

	informed := 0.0

	if adverse != nil && adverse.Sign() > 0 && row.ExpectedSpread != nil &&
		row.ExpectedSpread.Sign() > 0 {
		informed = adverse.SetScale(8).Div(row.ExpectedSpread.SetScale(8)).Float64()
	}

	if math.IsNaN(informed) || math.IsInf(informed, 0) || informed < 0 || informed > 1 {
		return 0, "", false
	}

	penalty := math.Max(scarcity, math.Max(hollow, informed))
	reasons := make([]string, 0, 3)

	if scarcity > 0 {
		reasons = append(reasons, "executable-depth scarcity")
	}

	if hollow > 0 {
		reasons = append(reasons, "toxicity")
	}

	if informed > 0 {
		reasons = append(reasons, "adverse selection")
	}

	if penalty == 0 {
		return 0, "clean executable liquidity and order flow", true
	}

	return penalty / (1 + penalty),
		"overlap max: " + strings.Join(reasons, " + "), true
}

func latestMeasurement(
	thesis *types.Thesis,
	symbol string,
	source types.SourceType,
) (*types.Measurement, bool) {
	series := thesis.Series(symbol)

	for index := len(series) - 1; index >= 0; index-- {
		if series[index].Source == source {
			return series[index], true
		}
	}

	return nil, false
}

func rawMetric(
	measurement *types.Measurement,
	metric types.MetricType,
	side types.MeasurementSide,
) (float64, bool) {
	if measurement == nil {
		return 0, false
	}

	sample, ok := measurement.Metrics[types.MetricKey(metric, side)]

	if !ok || math.IsNaN(sample.Raw) || math.IsInf(sample.Raw, 0) {
		return 0, false
	}

	return sample.Raw, true
}

/*
estimateImpact derives market impact from measured depth thinning or scarcity.
*/
func estimateImpact(
	thesis *types.Thesis, symbol string, spread *decimal.Decimal,
) *decimal.Decimal {
	if spread == nil || spread.Sign() <= 0 {
		return decimal.NewFromInt64(0)
	}

	impactRatio := 0.05

	rows, ok := thesis.Measurements.Load(types.SourceLiquidity)

	if ok && rows != nil {
		for _, measurement := range rows.([]*types.Measurement) {
			if measurement != nil && measurement.Symbol == symbol {
				scarcity := getMetricValue(measurement, string(types.MetricKey(types.MetricScarcityScore, types.SideNone)))
				if scarcity > 0 {
					impactRatio = math.Max(impactRatio, 0.05+0.25*math.Min(1.0, scarcity))
				}
			}
		}
	}

	rows, ok = thesis.Measurements.Load(types.SourceDepthFlow)

	if ok && rows != nil {
		for _, measurement := range rows.([]*types.Measurement) {
			if measurement != nil && measurement.Symbol == symbol {
				thin := getMetricValue(measurement, string(types.MetricKey(types.MetricThinScore, types.SideNone)))
				if thin > 0 {
					impactRatio = math.Max(impactRatio, 0.05+0.25*math.Min(1.0, thin))
				}
			}
		}
	}

	return spread.SetScale(8).Mul(decimal.NewFromFloat64(impactRatio))
}

/*
hollowPressure reads how much of the bid touch is being cancelled rather than
filled, as a share of what was resting there before it went.

The bid side is the one that matters to a long: it is the liquidity the position
would have to sell into, and a best price holding steady while the size behind
it evaporates is a quote that is not for sale.

Cancellation is the right metric and retreat is not. Toxicity emits a buy-side
retreat when the bid steps *down*, which produces no new peak for a regulator to
suppress; it emits a cancellation when size disappears at an unchanged best
price, which is exactly the hollow quote that would otherwise mint a peak the
position could never have sold into.

The raw cancellation is a quantity, so it is normalised here against the touch
it was pulled from — the signal publishes a normalised share for retreat but not
for cancellation.

The second return distinguishes "nothing cancelled" from "no reading", so the
regulator can tell an all-clear from silence.
*/
func hollowPressure(thesis *types.Thesis, symbol string) (float64, bool) {
	pressure := 0.0
	observed := false

	thesis.Measurements.Range(func(key, value any) bool {
		rows, ok := value.([]*types.Measurement)

		if !ok {
			measurement, single := value.(*types.Measurement)

			if !single || measurement == nil {
				return true
			}

			rows = []*types.Measurement{measurement}
		}

		for _, measurement := range rows {
			if measurement == nil || measurement.Symbol != symbol ||
				measurement.Source != types.SourceToxicity {
				continue
			}

			cancelled, cancelledReady := measurement.Metrics[types.MetricKey(
				types.MetricCancelledQuantity, types.SideBuy,
			)]
			resting, restingReady := measurement.Metrics[types.MetricKey(
				types.MetricTouchQuantity, types.SideBuy,
			)]

			if !cancelledReady || !restingReady {
				continue
			}

			observed = true

			if math.IsNaN(cancelled.Raw) || math.IsInf(cancelled.Raw, 0) ||
				cancelled.Raw <= 0 {
				continue
			}

			prior := resting.Raw + cancelled.Raw

			if math.IsNaN(prior) || math.IsInf(prior, 0) || prior <= 0 {
				continue
			}

			pressure = math.Max(pressure, cancelled.Raw/prior)
		}

		return true
	})

	return pressure, observed
}

func getCausalMetrics(thesis *types.Thesis, symbol string) (doExp, uplift, noise float64, ready bool) {
	if thesis == nil || thesis.Causal == nil {
		return 0, 0, 0, false
	}

	val, ok := thesis.Causal.Load(symbol)

	if !ok || val == nil {
		return 0, 0, 0, false
	}

	m, ok := val.(map[string]any)

	if !ok {
		return 0, 0, 0, false
	}

	doExp = getFloat(m, "doExpectation")
	uplift = getFloat(m, "uplift")
	noise = getFloat(m, "noise")
	strength := getFloat(m, "strength")
	confidence := getFloat(m, "confidence")

	ready = (strength > 0 || confidence > 0) && (uplift != 0 || doExp != 0)
	return doExp, uplift, noise, ready
}

func getCognition(thesis *types.Thesis, symbol string) types.Cognition {
	if thesis == nil || thesis.Cognition == nil {
		return types.Cognition{}
	}

	val, ok := thesis.Cognition.Load(symbol)

	if !ok || val == nil {
		return types.Cognition{}
	}

	if cog, ok := val.(types.Cognition); ok {
		return cog
	}

	return types.Cognition{}
}

func inspectGraph(thesis *types.Thesis, symbol string) (supports, contradicts float64, hasGraph bool) {
	if thesis == nil || thesis.Graphs == nil {
		return 0, 0, false
	}

	val, ok := thesis.Graphs.Load("market_graph")

	if !ok || val == nil {
		return 0, 0, false
	}

	g, ok := val.(*graph.Graph)

	if !ok || g == nil {
		return 0, 0, false
	}

	for _, edge := range g.Edges {
		fromNode, fromOk := g.Nodes[edge.From]
		toNode, toOk := g.Nodes[edge.To]

		if (fromOk && fromNode.Symbol == symbol) || (toOk && toNode.Symbol == symbol) {
			vote := math.Tanh(math.Abs(edge.Weight)) * math.Abs(edge.Confidence)

			switch edge.Relation {
			case graph.RelationSupports,
				graph.RelationConditions,
				graph.RelationLeads:
				supports += vote
			case graph.RelationContradicts:
				contradicts += vote
			}
		}
	}

	return supports, contradicts, true
}

func usableCausalRows(rows [][]float64) int {
	usable := 0

	for _, row := range rows {
		if len(row) >= causalRowWidth {
			usable++
		}
	}

	return usable
}

func getCausalHistoryRows(thesis *types.Thesis, symbol string) [][]float64 {
	if thesis == nil || thesis.Causal == nil {
		return nil
	}

	val, ok := thesis.Causal.Load(symbol)

	if !ok || val == nil {
		return nil
	}

	m, ok := val.(map[string]any)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"causal history invalid: expected map[string]any",
			nil,
		))

		return nil
	}

	if rowsRaw, ok := m["historyRows"].([][]float64); ok {
		return rowsRaw
	}

	return nil
}

func resonanceReading(thesis *types.Thesis, symbol string) (types.ResonanceReading, bool) {
	if thesis == nil || thesis.Resonance == nil || symbol == "" {
		return types.ResonanceReading{}, false
	}

	raw, found := thesis.Resonance.Load(symbol)

	if !found || raw == nil {
		return types.ResonanceReading{}, false
	}

	reading, ok := raw.(types.ResonanceReading)

	if !ok {
		return types.ResonanceReading{}, false
	}

	return reading, true
}

func getFloat(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}

	return 0.0
}
