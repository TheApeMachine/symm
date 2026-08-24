package strategy

import (
	"time"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/relation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
DefaultRelationPlans returns the initial explicit candidate Relation space.
Eligibility is structural only: within-symbol coordinate pairs selected from
every signal source, targeting the outcome and the main flow variable. No
evidence threshold ever decides eligibility; the plan is the candidate
space, and the Relation layer measures whatever the data supports.
*/
func DefaultRelationPlans(epoch uint64) []*relation.RelationPlan {
	return []*relation.RelationPlan{{
		Version: 1,
		Epoch:   epoch,
		Symbol:  "",
		// Sources are the parent-candidate coordinates of the schema (one or
		// two per signal), Targets are the coordinates whose future they may
		// predict. The cross product is the explicit same-symbol candidate
		// space; self-pairs are excluded at estimation time.
		Sources: defaultMarketCoordinates(epoch),
		Targets: []relation.Selector{
			{Source: "cvd", Metric: "midpoint_log_return"},
			{Source: "cvd", Metric: "signed_net_fraction_zscore"},
		},
		Lag: relation.LagDomain{MaxLag: 30 * time.Second},
	}}
}

/*
defaultMarketCoordinates returns the explicit parent-candidate coordinate
selectors from every signal source. This is the initial candidate market
variable set; it is data, not code — widening it does not require a code
change. Coordinates not listed here still enter the observation store and
remain available for other queries.
*/
func defaultMarketCoordinates(epoch uint64) []relation.Selector {
	return []relation.Selector{
		{Source: "cvd", Metric: "signed_net_fraction_zscore"},
		{Source: "cvd", Metric: "gross_notional_rate_zscore"},
		{Source: "hawkes", Metric: "conditional_intensity", Side: "buy"},
		{Source: "hawkes", Metric: "branching_spectral_radius"},
		{Source: "liquidity", Metric: "touch_notional_imbalance"},
		{Source: "depthflow", Metric: "book_imbalance"},
		{Source: "depthflow", Metric: "touch_imbalance"},
		{Source: "toxicity", Metric: "withdrawal_fraction_zscore", Side: "bid"},
		{Source: "toxicity", Metric: "fill_fraction_zscore", Side: "bid"},
		{Source: "derivatives", Metric: "open_interest_growth_zscore"},
		{Source: "derivatives", Metric: "basis_zscore"},
		{Source: "correlation", Metric: "signed_correlation"},
		{Source: "leadlag", Metric: "best_lag_correlation"},
		{Source: "exhaust", Metric: "book_imbalance_zscore"},
		{Source: "exhaust", Metric: "spread_zscore"},
		{Source: "sentiment", Metric: "advance_count"},
	}
}

/*
DefaultCausalSchema returns the initial symbol-agnostic CausalSchema. Every
variable is an actual Measurement coordinate with its full typed identity
(unit and timescale included), so schema variables resolve exactly against
the observational store. The schema authorizes which structural directions
are allowed; the Relation layer supplies the measured temporal relationship
(which relationships exist and their lags). The fixed semantic frame (flow,
hawkes, coherence, regime, confidence) plays no role.
*/
func DefaultCausalSchema(epoch uint64) *causal.CausalSchema {
	schema := causal.NewCausalSchema("market-v1", "", epoch)

	priceReturn := causal.VariableID{
		Coordinate: marketCoordinate("cvd", "midpoint_log_return", ""),
		Role:       causal.RoleMarket,
	}
	flow := causal.VariableID{
		Coordinate: marketCoordinate("cvd", "signed_net_fraction_zscore", ""),
		Role:       causal.RoleMarket,
	}
	grossRate := causal.VariableID{
		Coordinate: marketCoordinate("cvd", "gross_notional_rate_zscore", ""),
		Role:       causal.RoleMarket,
	}
	hawkesBuy := causal.VariableID{
		Coordinate: marketCoordinate("hawkes", "conditional_intensity", "buy"),
		Role:       causal.RoleMarket,
	}

	parents := make([]causal.AllowedParent, 0, len(defaultMarketCoordinates(epoch)))

	for _, selector := range defaultMarketCoordinates(epoch) {
		coordinate := marketCoordinate(selector.Source, selector.Metric, selector.Side)

		if coordinate == priceReturn.Coordinate {
			continue
		}

		parents = append(parents, causal.AllowedParent{
			Parent:   marketVariable(coordinate),
			Lag:      time.Second,
			LagSource: "schema",
		})
	}

	// The outcome: price return depends on its own history and the
	// schema-authorized market variables from every signal.
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: priceReturn,
		SelfLag:  time.Second,
		Parents:  parents,
	})

	// The signed-flow coordinate: driven by Hawkes buy intensity and gross
	// flow — the structural chain Hawkes → Flow → Price.
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: flow,
		SelfLag:  time.Second,
		Parents: []causal.AllowedParent{
			{Parent: hawkesBuy, Lag: time.Second, LagSource: "schema"},
			{Parent: grossRate, Lag: time.Second, LagSource: "schema"},
		},
	})

	// Every other market variable evolves with its own history (self-lag
	// only), so multi-step rollouts advance the whole system rather than
	// freezing the parents.
	for _, selector := range defaultMarketCoordinates(epoch) {
		coordinate := marketCoordinate(selector.Source, selector.Metric, selector.Side)

		if coordinate == flow.Coordinate || coordinate == priceReturn.Coordinate {
			continue
		}

		schema.AddMarketVariable(causal.MarketVariable{
			Variable: marketVariable(coordinate),
			SelfLag:  time.Second,
		})
	}

	position := causal.VariableID{
		Coordinate: marketCoordinate("portfolio", "position", ""),
		Role:       causal.RolePortfolio,
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
marketVariable wraps a coordinate in the market role.
*/
func marketVariable(coordinate relation.Coordinate) causal.VariableID {
	return causal.VariableID{Coordinate: coordinate, Role: causal.RoleMarket}
}

/*
marketCoordinate builds the full typed identity of one market coordinate.
Identity is exact: unit and timescale participate, so a schema variable only
resolves to stored observations that carry the same identity.
*/
func marketCoordinate(source string, metric string, side string) relation.Coordinate {
	return relation.Coordinate{
		Source:    source,
		Metric:    metric,
		Side:      side,
		Unit:      nmtypes.UnitDimensionless,
		Timescale: nmtypes.TimescaleInstantaneous,
	}
}
