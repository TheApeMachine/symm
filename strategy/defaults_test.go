package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/causal"
)

/*
parentMetricKeys returns the structural (source, metric) set of a variable's
schema-authorized parents, ignoring side, so tests assert wiring independent
of side qualification.
*/
func parentMetricKeys(schema *causal.CausalSchema, variable causal.VariableID) map[string]bool {
	keys := map[string]bool{}

	for _, marketVariable := range schema.MarketVariables {
		if marketVariable.Variable != variable {
			continue
		}

		for _, parent := range marketVariable.Parents {
			keys[parent.Parent.Coordinate.Source+"/"+parent.Parent.Coordinate.Metric] = true
		}

		return keys
	}

	return keys
}

func TestDefaultCausalSchema(t *testing.T) {
	Convey("Given the default market schema", t, func() {
		schema := DefaultCausalSchema(1, time.Second)
		priceReturn := marketVariable(marketCoordinate(catalogSpec("cvd", "midpoint_log_return", "")))
		signedFlow := marketVariable(marketCoordinate(catalogSpec("cvd", "signed_net_fraction_zscore", "")))
		grossRate := marketVariable(marketCoordinate(catalogSpec("cvd", "gross_notional_rate_zscore", "")))
		hawkesBuy := marketVariable(marketCoordinate(catalogSpec("hawkes", "conditional_intensity", "buy")))

		Convey("every catalog variable is registered exactly once", func() {
			So(len(schema.MarketVariables), ShouldEqual, len(defaultMarketCatalog))

			seen := map[causal.VariableID]bool{}

			for _, marketVariable := range schema.MarketVariables {
				So(seen[marketVariable.Variable], ShouldBeFalse)
				seen[marketVariable.Variable] = true
			}
		})

		Convey("the outcome is the single terminal variable", func() {
			So(len(schema.Outcomes), ShouldEqual, 1)
			So(schema.Outcomes[0], ShouldEqual, priceReturn)
		})

		Convey("the cascade is mediated, not a flat all-to-price star", func() {
			priceParents := parentMetricKeys(schema, priceReturn)

			// The outcome's only direct structural parents are the Layer-3
			// flow/toxicity/leadlag signals; the upstream macro variables
			// (hawkes, depthflow, liquidity, derivatives, correlation,
			// sentiment, pumpdump, exhaustion) are NOT direct parents.
			for _, forbidden := range []string{
				"hawkes/conditional_intensity",
				"hawkes/branching_spectral_radius",
				"hawkes/background_rate",
				"depthflow/book_imbalance",
				"depthflow/touch_imbalance",
				"depthflow/book_turnover_rate",
				"depthflow/imbalance_resolution_gap",
				"liquidity/touch_notional_imbalance",
				"liquidity/relative_spread",
				"derivatives/basis_zscore",
				"derivatives/open_interest_growth_zscore",
				"derivatives/liquidation_notional_rate",
				"correlation/signed_correlation",
				"sentiment/directional_consensus",
				"sentiment/breadth_zscore",
				"pumpdump/relative_spread",
				"exhaustion/book_imbalance_zscore",
				"exhaustion/spread_zscore",
				"toxicity/withdrawal_fraction_zscore",
			} {
				So(priceParents[forbidden], ShouldBeFalse)
			}

			// The mediated flow/toxicity signals and leadlag are direct outcome
			// drivers; withdrawal does not reach price directly.
			So(priceParents["cvd/signed_net_fraction_zscore"], ShouldBeTrue)
			So(priceParents["toxicity/fill_fraction_zscore"], ShouldBeTrue)
			So(priceParents["toxicity/retreat_rate"], ShouldBeTrue)
			So(priceParents["leadlag/best_lag_correlation"], ShouldBeTrue)
		})

		Convey("flow sits between the book state and the outcome", func() {
			flowParents := parentMetricKeys(schema, signedFlow)

			So(flowParents["depthflow/book_imbalance"], ShouldBeTrue)
			So(flowParents["liquidity/touch_notional_imbalance"], ShouldBeTrue)
			So(flowParents["hawkes/conditional_intensity"], ShouldBeTrue)

			// Flow does not reach the outcome directly from its own layer;
			// it is a parent of the outcome, not the other way around.
			grossParents := parentMetricKeys(schema, grossRate)
			So(grossParents["hawkes/conditional_intensity"], ShouldBeTrue)
			So(grossParents["depthflow/book_turnover_rate"], ShouldBeTrue)

			hawkesParents := parentMetricKeys(schema, hawkesBuy)
			So(hawkesParents["hawkes/branching_spectral_radius"], ShouldBeTrue)
			So(hawkesParents["depthflow/book_imbalance"], ShouldBeTrue)
		})

		Convey("the relation plan mirrors the schema wiring", func() {
			plans := RelationPlansFromSchema(schema, 1, 30*time.Second)
			So(len(plans), ShouldEqual, 1)

			expectedPairs := 0

			for _, marketVariable := range schema.MarketVariables {
				expectedPairs += len(marketVariable.Parents)
			}

			So(len(plans[0].Pairs), ShouldEqual, expectedPairs)
			So(expectedPairs, ShouldBeGreaterThan, 30)
		})
	})
}
