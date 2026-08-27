package strategy

import (
	"fmt"
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
	// Tier 1: Latent Arrival, Cross-Asset & Macro State
	{Selector: relation.Selector{Source: "hawkes", Metric: "branching_spectral_radius"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "hawkes", Metric: "background_rate"}, Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerSecond},
	{Selector: relation.Selector{Source: "derivatives", Metric: "basis_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "derivatives", Metric: "open_interest_growth_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "derivatives", Metric: "liquidation_notional_rate"}, Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerSecond},
	{Selector: relation.Selector{Source: "leadlag", Metric: "best_lag_correlation"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "leadlag", Metric: "best_lag_seconds"}, Unit: nmtypes.UnitSecond, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "correlation", Metric: "signed_correlation"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "sentiment", Metric: "directional_consensus"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "sentiment", Metric: "breadth_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},

	// Tier 2: Displayed Liquidity & Book Morphology (Passive Intent)
	{Selector: relation.Selector{Source: "depthflow", Metric: "book_imbalance"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "depthflow", Metric: "touch_imbalance"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "depthflow", Metric: "book_turnover_rate"}, Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerSecond},
	{Selector: relation.Selector{Source: "depthflow", Metric: "imbalance_resolution_gap"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "liquidity", Metric: "touch_notional_imbalance"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "liquidity", Metric: "relative_spread"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "exhaustion", Metric: "book_imbalance_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "exhaustion", Metric: "spread_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "pumpdump", Metric: "relative_spread"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},

	// Tier 3: Aggressive Execution, Fill Dynamics & Toxicity (Active Flow)
	{Selector: relation.Selector{Source: "hawkes", Metric: "conditional_intensity", Side: "buy"}, Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerSecond},
	{Selector: relation.Selector{Source: "hawkes", Metric: "conditional_intensity", Side: "sell"}, Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerSecond},
	{Selector: relation.Selector{Source: "cvd", Metric: "signed_net_fraction_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "cvd", Metric: "gross_notional_rate_zscore"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "toxicity", Metric: "withdrawal_fraction_zscore", Side: "bid"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "toxicity", Metric: "withdrawal_fraction_zscore", Side: "ask"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "toxicity", Metric: "fill_fraction_zscore", Side: "bid"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "toxicity", Metric: "fill_fraction_zscore", Side: "ask"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "toxicity", Metric: "retreat_rate", Side: "ask"}, Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescaleInstantaneous},

	// Tier 4: Outcome & Response Mechanics
	{Selector: relation.Selector{Source: "cvd", Metric: "midpoint_log_return"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	{Selector: relation.Selector{Source: "cvd", Metric: "flow_aligned_midpoint_return"}, Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
}

/*
DefaultCausalSchema returns the initial symbol-agnostic CausalSchema as an
explicit four-tier mediated DAG. Every variable is an actual Measurement
coordinate with its full typed identity (unit and timescale included),
declared explicitly in the market catalog, so schema variables resolve exactly
against the observational store. The schema authorizes which structural
directions are allowed; the Relation layer supplies the measured temporal
relationship (which relationships exist and their lags). step is the configured
measurement step duration used for the schema's structural self-lag
declarations.

Information flows naturally down the cascade instead of through the flat
all-to-price star that double-counted every signal as a direct driver of
return:

	Tier 1 (latent / macro / external, autonomous): hawkes branching radius &
	    background rate, derivatives basis/OI/liquidations, leadlag, correlation,
	    sentiment.
	Tier 2 (displayed liquidity / passive book morphology): depthflow,
	    liquidity, exhaustion, pumpdump spreads.
	Tier 3 (aggressive execution / flow / toxicity precursors): hawkes buy &
	    sell intensity, cvd signed & gross flow, toxicity withdrawal/fill
	    fractions and ask retreat rate.
	Tier 4 (settlement / outcome): cvd midpoint_log_return and its
	    flow-aligned twin.

Every tier-2/3/4 metric lists exactly the structural parents that mediate
information down to it; no variable is wired as a direct parent of the outcome
unless the cascade reaches it that way. The one intentional exception is the
Tier 1 leadlag/best_lag_correlation variable, which is wired directly to the
Tier 4 outcome, bypassing Tiers 2 and 3.
*/
func DefaultCausalSchema(epoch uint64, step time.Duration) *causal.CausalSchema {
	if step <= 0 {
		step = time.Second
	}

	schema := causal.NewCausalSchema("market-v2-precursor", "", epoch)

	// Resolve a typed catalog coordinate into its market variable identity.
	variable := func(source, metric, side string) causal.VariableID {
		spec := catalogSpec(source, metric, side)

		if spec == (MarketCoordinateSpec{}) {
			panic(fmt.Sprintf(
				"strategy: unresolved catalog coordinate for source=%s metric=%s side=%s",
				source, metric, side,
			))
		}

		return marketVariable(marketCoordinate(spec))
	}

	// 1. Tier 1: Latent / Macro / External (Self-Lag Only)
	tier1 := []causal.VariableID{
		variable("hawkes", "branching_spectral_radius", ""),
		variable("hawkes", "background_rate", ""),
		variable("derivatives", "basis_zscore", ""),
		variable("derivatives", "open_interest_growth_zscore", ""),
		variable("derivatives", "liquidation_notional_rate", ""),
		variable("leadlag", "best_lag_correlation", ""),
		variable("leadlag", "best_lag_seconds", ""),
		variable("correlation", "signed_correlation", ""),
		variable("sentiment", "directional_consensus", ""),
		variable("sentiment", "breadth_zscore", ""),
	}

	for _, varID := range tier1 {
		schema.AddMarketVariable(causal.MarketVariable{Variable: varID, SelfLag: step})
	}

	// 2. Tier 2: Passive Orderbook & Liquidity Dynamics
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("depthflow", "book_imbalance", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("derivatives", "basis_zscore", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("leadlag", "best_lag_correlation", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("hawkes", "branching_spectral_radius", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("depthflow", "touch_imbalance", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("depthflow", "book_imbalance", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("depthflow", "book_turnover_rate", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("hawkes", "background_rate", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("derivatives", "liquidation_notional_rate", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("depthflow", "imbalance_resolution_gap", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("depthflow", "book_imbalance", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("depthflow", "touch_imbalance", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("liquidity", "touch_notional_imbalance", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("leadlag", "best_lag_correlation", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("correlation", "signed_correlation", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("liquidity", "relative_spread", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("hawkes", "branching_spectral_radius", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("derivatives", "basis_zscore", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("exhaustion", "book_imbalance_zscore", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("depthflow", "book_imbalance", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("exhaustion", "spread_zscore", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("liquidity", "relative_spread", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("sentiment", "breadth_zscore", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("pumpdump", "relative_spread", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("liquidity", "relative_spread", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("derivatives", "basis_zscore", ""), Lag: step, LagSource: "schema"},
		},
	})

	// 3. Tier 3: Active Execution, Fill Dynamics & Toxicity (Precursors)
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("hawkes", "conditional_intensity", "buy"),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("hawkes", "branching_spectral_radius", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("depthflow", "book_imbalance", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("derivatives", "open_interest_growth_zscore", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("hawkes", "conditional_intensity", "sell"),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("hawkes", "branching_spectral_radius", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("depthflow", "book_imbalance", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("derivatives", "liquidation_notional_rate", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("cvd", "gross_notional_rate_zscore", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("hawkes", "conditional_intensity", "buy"), Lag: step, LagSource: "schema"},
			{Parent: variable("hawkes", "conditional_intensity", "sell"), Lag: step, LagSource: "schema"},
			{Parent: variable("depthflow", "book_turnover_rate", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("cvd", "signed_net_fraction_zscore", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("depthflow", "book_imbalance", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("liquidity", "touch_notional_imbalance", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("hawkes", "conditional_intensity", "buy"), Lag: step, LagSource: "schema"},
			{Parent: variable("hawkes", "conditional_intensity", "sell"), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("toxicity", "withdrawal_fraction_zscore", "bid"),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("cvd", "signed_net_fraction_zscore", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("depthflow", "touch_imbalance", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("toxicity", "withdrawal_fraction_zscore", "ask"),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("cvd", "signed_net_fraction_zscore", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("depthflow", "touch_imbalance", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("toxicity", "fill_fraction_zscore", "bid"),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("cvd", "gross_notional_rate_zscore", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("liquidity", "touch_notional_imbalance", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("toxicity", "fill_fraction_zscore", "ask"),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("cvd", "gross_notional_rate_zscore", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("liquidity", "touch_notional_imbalance", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("toxicity", "retreat_rate", "ask"),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("hawkes", "conditional_intensity", "buy"), Lag: step, LagSource: "schema"},
			{Parent: variable("depthflow", "book_imbalance", ""), Lag: step, LagSource: "schema"},
		},
	})

	// 4. Tier 4: Settlement Outcome
	priceReturn := variable("cvd", "midpoint_log_return", "")
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: priceReturn,
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: variable("cvd", "signed_net_fraction_zscore", ""), Lag: step, LagSource: "schema"},
			{Parent: variable("toxicity", "retreat_rate", "ask"), Lag: step, LagSource: "schema"},
			{Parent: variable("toxicity", "fill_fraction_zscore", "bid"), Lag: step, LagSource: "schema"},
			{Parent: variable("toxicity", "fill_fraction_zscore", "ask"), Lag: step, LagSource: "schema"},
			{Parent: variable("leadlag", "best_lag_correlation", ""), Lag: step, LagSource: "schema"},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: variable("cvd", "flow_aligned_midpoint_return", ""),
		SelfLag:  step,
		Parents: []causal.AllowedParent{
			{Parent: priceReturn, Lag: step, LagSource: "schema"},
			{Parent: variable("cvd", "signed_net_fraction_zscore", ""), Lag: step, LagSource: "schema"},
		},
	})

	// 5. Portfolio Interventions & Outcome Registration
	position := causal.VariableID{
		Coordinate: relation.Coordinate{
			Source:    "portfolio",
			Metric:    "position",
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		},
		Role: causal.RolePortfolio,
	}

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
