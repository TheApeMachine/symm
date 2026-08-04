package strategy

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
estimateImpact derives market impact from measured depth thinning or scarcity.
*/
func estimateImpact(thesis *types.Thesis, symbol string, spread *decimal.Decimal) *decimal.Decimal {
	if spread == nil || spread.Sign() <= 0 {
		return decimal.NewFromInt64(0)
	}

	impactRatio := 0.05

	if thesis != nil && thesis.Measurements != nil {
		loadMeasurements := func(source types.SourceType) []*types.Measurement {
			if val, ok := thesis.Measurements.Load(source); ok {
				if ms, ok := val.([]*types.Measurement); ok {
					return ms
				}
			}
			if val, ok := thesis.Measurements.Load(string(source)); ok {
				if ms, ok := val.([]*types.Measurement); ok {
					return ms
				}
			}
			return nil
		}

		for _, measurement := range loadMeasurements(types.SourceLiquidity) {
			if measurement != nil && measurement.Symbol == symbol {
				scarcity := getMetricValue(measurement, string(types.MetricKey(types.MetricScarcityScore, types.SideNone)))
				if scarcity > 0 {
					impactRatio = math.Max(impactRatio, 0.05+0.25*math.Min(1.0, scarcity))
				}
			}
		}

		for _, measurement := range loadMeasurements(types.SourceDepthFlow) {
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
	if thesis == nil || thesis.Measurements == nil {
		return 0, false
	}

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

			observed = true

			cancelled := measurement.Sample(
				types.MetricCancelledQuantity, types.SideBuy,
			).Raw
			resting := measurement.Sample(
				types.MetricTouchQuantity, types.SideBuy,
			).Raw

			if math.IsNaN(cancelled) || math.IsInf(cancelled, 0) || cancelled <= 0 {
				continue
			}

			prior := resting + cancelled

			if math.IsNaN(prior) || math.IsInf(prior, 0) || prior <= 0 {
				continue
			}

			pressure = math.Max(pressure, cancelled/prior)
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
		return nil
	}

	if rowsRaw, ok := m["historyRows"].([][]float64); ok {
		return rowsRaw
	}

	return nil
}

func loadResonanceFloat(row map[string]any, key string) (float64, bool) {
	raw, found := row[key]

	if !found {
		return 0, false
	}

	value, ok := raw.(float64)

	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}

	return value, true
}

func resonanceRow(thesis *types.Thesis, symbol string) (map[string]any, bool) {
	if thesis == nil || thesis.Resonance == nil || symbol == "" {
		return nil, false
	}

	raw, found := thesis.Resonance.Load(symbol)

	if !found {
		return nil, false
	}

	row, ok := raw.(map[string]any)

	if !ok {
		return nil, false
	}

	return row, true
}

func accumulatedReturn(curve []float64, surviving []float64) float64 {
	total := 0.0
	reference := 0.0

	if len(surviving) > 0 {
		reference = surviving[0]
	}

	for index, step := range curve {
		if math.IsNaN(step) || math.IsInf(step, 0) {
			continue
		}

		weight := 1.0

		if index < len(surviving) && reference > 0 {
			weight = math.Min(1, surviving[index]/reference)
		}

		if weight <= 0 {
			break
		}

		total += step * weight
	}

	return total
}

func retention(row map[string]any, curveLength int) []float64 {
	if raw, found := row["forwardRetention"]; found {
		if surviving, ok := raw.([]float64); ok && len(surviving) == curveLength {
			return surviving
		}
	}

	surviving := make([]float64, curveLength)

	for index := range surviving {
		surviving[index] = 1
	}

	return surviving
}

func horizon(row map[string]any, curveLength int) uint64 {
	if raw, found := row["activeHorizon"]; found {
		if active, ok := raw.(int); ok && active > 0 {
			return uint64(active)
		}
	}

	if curveLength > 0 {
		return uint64(curveLength)
	}

	return 1
}

func getFloat(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}

	return 0.0
}
