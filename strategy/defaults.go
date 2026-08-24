package strategy

import (
	"time"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/relation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
DefaultRelationPlans returns the initial explicit Relation plan. Eligibility
is structural only: within-symbol pairs of exact coordinates with exact
controls. No evidence threshold ever decides eligibility.
*/
func DefaultRelationPlans(epoch uint64) []*relation.RelationPlan {
	return []*relation.RelationPlan{{
		Version: 1,
		Epoch:   epoch,
		Symbol:  "",
		Pairs: []relation.PlannedPair{{
			Source: relation.Selector{Source: "cvd", Metric: "signed_net_fraction_zscore"},
			Target: relation.Selector{Source: "cvd", Metric: "midpoint_log_return"},
		}},
		Controls: []relation.ControlSelector{{
			Selector: relation.Selector{Source: "cvd", Metric: "gross_notional_rate_zscore"},
		}},
		Lag: relation.LagDomain{MaxLag: 30 * time.Second},
	}}
}

/*
DefaultCausalSchema returns the initial symbol-agnostic CausalSchema. Every
variable is an actual Measurement coordinate with its full typed identity
(unit and timescale included), so schema variables resolve exactly against
the observational store; the fixed semantic frame (flow, hawkes, coherence,
regime, confidence) plays no role here. The schema explicitly authorizes the
market transition directions and the portfolio action boundary.
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

	// Market transition: each market variable depends on its own history and
	// explicitly allowed lagged market parents. Future-to-past edges are
	// forbidden structurally (parents are always lagged).
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: priceReturn,
		SelfLag:  time.Second,
		Parents: []causal.AllowedParent{
			{Parent: flow, Lag: time.Second},
			{Parent: grossRate, Lag: time.Second},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: flow,
		SelfLag:  time.Second,
		Parents: []causal.AllowedParent{
			{Parent: grossRate, Lag: time.Second},
		},
	})
	schema.AddMarketVariable(causal.MarketVariable{
		Variable: grossRate,
		SelfLag:  time.Second,
	})

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
