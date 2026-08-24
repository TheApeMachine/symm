package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/relation"
)

/*
testVariable builds one schema variable identity for tests. It is shared by
the schema, model, and identification test files.
*/
func testVariable(source string, metric string) VariableID {
	return VariableID{
		Coordinate: relation.Coordinate{
			Symbol: "BTC/USD",
			Source: source,
			Metric: metric,
			Epoch:  1,
		},
		Role: RoleMarket,
	}
}

func testSchema() *CausalSchema {
	schema := NewCausalSchema("test-schema", "BTC/USD", 1)
	priceReturn := testVariable("cvd", "midpoint_log_return")
	cvdFlow := testVariable("cvd", "signed_net_fraction_zscore")
	hawkes := testVariable("hawkes", "conditional_intensity:buy")

	schema.AddMarketVariable(MarketVariable{
		Variable: priceReturn,
		SelfLag:  time.Second,
		Parents: []AllowedParent{
			{Parent: cvdFlow, Lag: time.Second},
		},
	})
	schema.AddMarketVariable(MarketVariable{
		Variable: cvdFlow,
		SelfLag:  time.Second,
		Parents: []AllowedParent{
			{Parent: hawkes, Lag: time.Second},
		},
	})

	position := VariableID{
		Coordinate: relation.Coordinate{Symbol: "BTC/USD", Source: "portfolio", Metric: "position", Epoch: 1},
		Role:       RolePortfolio,
	}
	schema.AddAction(ActionDefinition{Name: "enter", Variable: position})
	schema.AddAction(ActionDefinition{Name: "exit", Variable: position})
	schema.AddAction(ActionDefinition{Name: "scale", Variable: position})
	schema.AddPortfolioVariable(position)
	schema.AddOutcome(priceReturn)
	schema.ForbidDirection(priceReturn, hawkes)

	return schema
}

func TestCausalSchemaConformance(t *testing.T) {
	Convey("Given a causal schema", t, func() {
		schema := testSchema()

		Convey("matrix materialization is reversible", func() {
			store := buildStore()
			matrix, err := schema.Materialize(store, time.Unix(0, 119*int64(time.Second)))
			So(err, ShouldBeNil)
			So(len(matrix.Index.Columns), ShouldEqual, 2)

			for column, variable := range matrix.Index.Columns {
				resolved, found := matrix.Index.VariableOf(column)
				So(found, ShouldBeTrue)
				So(resolved, ShouldEqual, variable)
				So(matrix.Index.ColumnOf(variable), ShouldEqual, column)
				So(variable.Coordinate.Source, ShouldNotBeBlank)
			}
		})

		Convey("future-direction rejection is explicit", func() {
			priceReturn := testVariable("cvd", "midpoint_log_return")
			hawkes := testVariable("hawkes", "conditional_intensity:buy")
			So(schema.DirectionForbidden(priceReturn, hawkes), ShouldBeTrue)
		})

		Convey("per-symbol binding stamps symbol and epoch on every variable", func() {
			bound := schema.ForSymbol("ETH/USD")

			for _, marketVariable := range bound.MarketVariables {
				So(marketVariable.Variable.Coordinate.Symbol, ShouldEqual, "ETH/USD")
				So(marketVariable.Variable.Coordinate.Epoch, ShouldEqual, schema.Epoch)
			}
		})
	})
}
