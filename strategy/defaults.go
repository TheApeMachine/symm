package strategy

import (
	"time"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/relation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
MarketCoordinateSpec is the explicit typed declaration of one market
coordinate: the structural selector plus the exact unit and timescale of the
measurement as emitted by its signal. Unit and timescale participate in the
coordinate identity, so the schema only ever asks the store for the identity
the signal actually produced. Roles and units are never inferred from metric
names.
*/
type MarketCoordinateSpec struct {
	Selector  relation.Selector
	Unit      nmtypes.Unit
	Timescale nmtypes.Timescale
}

/*
defaultMarketCatalog is the initial explicit candidate market variable set,
typed per coordinate. It spans every signal source, including pumpdump. It
is data, not code — widening it does not require a code change. Coordinates
not listed here still enter the observation store and remain available for
other queries.
*/
var defaultMarketCatalog = []MarketCoordinateSpec{
	{Selector: relation.Selector{Source: "cvd", Metric: "signed_net_fraction_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "cvd", Metric: "gross_notional_rate_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "cvd", Metric: "midpoint_log_return"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "hawkes", Metric: "conditional_intensity", Side: "buy"}, Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerSecond},
	{Selector: relation.Selector{Source: "hawkes", Metric: "branching_spectral_radius"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "liquidity", Metric: "touch_notional_imbalance"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "depthflow", Metric: "book_imbalance"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "depthflow", Metric: "touch_imbalance"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "toxicity", Metric: "withdrawal_fraction_zscore", Side: "bid"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "toxicity", Metric: "fill_fraction_zscore", Side: "bid"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "derivatives", Metric: "open_interest_growth_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "derivatives", Metric: "basis_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "correlation", Metric: "signed_correlation"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "leadlag", Metric: "best_lag_correlation"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "exhaust", Metric: "book_imbalance_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "exhaust", Metric: "spread_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "sentiment", Metric: "advance_count"}, Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "pumpdump", Metric: "relative_spread"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
}

/*
DefaultCausalSchema returns the initial symbol-agnostic CausalSchema. Every
variable is an actual Measurement coordinate with its full typed identity
(unit and timescale included), declared explicitly in the market catalog, so
schema variables resolve exactly against the observational store. The schema
authorizes which structural directions are allowed; the Relation layer
supplies the measured temporal relationship (which relationships exist and
their lags). The fixed semantic frame (flow, hawkes, coherence, regime,
confidence) plays no role. step is the configured measurement step duration
used for the schema's structural self-lag declarations.
*/
func DefaultCausalSchema(epoch uint64, step time.Duration) *causal.CausalSchema {
	if step <= 0 {
		step = time.Second
	}

	schema := causal.NewCausalSchema("market-v1", "", epoch)

	priceReturn := marketVariable(marketCoordinate(catalogSpec("cvd", "midpoint_log_return", "")))
	flow := marketVariable(marketCoordinate(catalogSpec("cvd", "signed_net_fraction_zscore", "")))
	grossRate := marketVariable(marketCoordinate(catalogSpec("cvd", "gross_notional_rate_zscore", "")))
	hawkesBuy := marketVariable(marketCoordinate(catalogSpec("hawkes", "conditional_intensity", "buy")))
	liquidityImbalance := marketVariable(marketCoordinate(catalogSpec("liquidity", "touch_notional_imbalance", "")))
	depthflowImbalance := marketVariable(marketCoordinate(catalogSpec("depthflow", "book_imbalance", "")))
	toxicityWithdrawal := marketVariable(marketCoordinate(catalogSpec("toxicity", "withdrawal_fraction_zscore", "bid")))

	// The outcome: price return depends on its own history and the
	// schema-authorized market variables from every signal.
	parents := make([]causal.AllowedParent, 0, len(defaultMarketCatalog))

	for _, spec := range defaultMarketCatalog {
		coordinate := marketCoordinate(spec)

		if coordinate == priceReturn.Coordinate {
			continue
		}

		parents = append(parents, causal.AllowedParent{
			Parent:    marketVariable(coordinate),
			Lag:       step,
			LagSource: "schema",
		})
	}

	schema.AddMarketVariable(causal.MarketVariable{
		Variable: priceReturn,
		SelfLag:  step,
		Parents:  parents,
	})

	// The signed-flow coordinate: driven by Hawkes buy intensity, gross
	// flow, and the microstructure coordinates — the structural chain
	// Liquidity/Depthflow/Toxicity/Hawkes → Flow → Price.
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: flow,
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: hawkesBuy, Lag: step, LagSource: "schema"},
			{Parent: grossRate, Lag: step, LagSource: "schema"},
			{Parent: liquidityImbalance, Lag: step, LagSource: "schema"},
			{Parent: depthflowImbalance, Lag: step, LagSource: "schema"},
			{Parent: toxicityWithdrawal, Lag: step, LagSource: "schema"},
		},
	})

	// Every other market variable evolves with its own history (self-lag
	// only), so multi-step rollouts advance the whole system rather than
	// freezing the parents.
	for _, spec := range defaultMarketCatalog {
		coordinate := marketCoordinate(spec)

		if coordinate == flow.Coordinate || coordinate == priceReturn.Coordinate {
			continue
		}

		schema.AddMarketVariable(causal.MarketVariable{
			Variable: marketVariable(coordinate),
			SelfLag:  step,
		})
	}

	position := causal.VariableID{
		Coordinate: relation.Coordinate{
			Source:    "portfolio",
			Metric:    "position",
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		},
		Role: causal.RolePortfolio,
	}

	// The critical action boundary: actions mutate portfolio variables only.
	schema.AddAction(causal.ActionDefinition{Name: "wait", Variable: position})
	schema.AddAction(causal.ActionDefinition{Name: "enter", Variable: position})
	schema.AddAction(causal.ActionDefinition{Name: "exit", Variable: position})
	schema.AddAction(causal.ActionDefinition{Name: "scale", Variable: position})
	schema.AddPortfolioVariable(position)
	schema.AddOutcome(priceReturn)

	return schema
}

/*
RelationPlansFromSchema generates the Relation plan from the schema's
authorized candidate edges: every schema-authorized parent direction of every
market variable becomes an explicit Source→Target pair. The schema is the
single source of wiring truth, so the candidate Relation space and the
causal graph cannot drift apart. maxLag is the explicit candidate lag search
window supplied by configuration or observed sampling cadence.
*/
func RelationPlansFromSchema(schema *causal.CausalSchema, epoch uint64, maxLag time.Duration) []*relation.RelationPlan {
	if schema == nil {
		return nil
	}

	if maxLag <= 0 {
		maxLag = 30 * time.Second
	}

	pairs := make([]relation.PlannedPair, 0)

	for _, marketVariable := range schema.MarketVariables {
		for _, parent := range marketVariable.Parents {
			pairs = append(pairs, relation.PlannedPair{
				Source: selectorOf(parent.Parent.Coordinate),
				Target: selectorOf(marketVariable.Variable.Coordinate),
			})
		}
	}

	return []*relation.RelationPlan{{
		Version: 1,
		Epoch:   epoch,
		Symbol:  "",
		Pairs:   pairs,
		Lag:     relation.LagDomain{MaxLag: maxLag},
	}}
}

/*
catalogSpec looks up the typed catalog entry for a coordinate selector,
returning a zero spec when absent.
*/
func catalogSpec(source string, metric string, side string) MarketCoordinateSpec {
	for _, spec := range defaultMarketCatalog {
		if spec.Selector.Source == source && spec.Selector.Metric == metric && spec.Selector.Side == side {
			return spec
		}
	}

	return MarketCoordinateSpec{}
}

/*
selectorOf projects a coordinate back to its structural selector.
*/
func selectorOf(coordinate relation.Coordinate) relation.Selector {
	return relation.Selector{
		Source: coordinate.Source,
		Metric: coordinate.Metric,
		Side:   coordinate.Side,
	}
}

/*
marketVariable wraps a coordinate in the market role.
*/
func marketVariable(coordinate relation.Coordinate) causal.VariableID {
	return causal.VariableID{Coordinate: coordinate, Role: causal.RoleMarket}
}

/*
marketCoordinate builds the full typed identity of one market coordinate
from its explicit catalog spec. Identity is exact: unit and timescale
participate, so a schema variable only resolves to stored observations that
carry the same identity.
*/
func marketCoordinate(spec MarketCoordinateSpec) relation.Coordinate {
	return relation.Coordinate{
		Source:    spec.Selector.Source,
		Metric:    spec.Selector.Metric,
		Side:      spec.Selector.Side,
		Unit:      spec.Unit,
		Timescale: spec.Timescale,
	}
}
