package logic_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestCategoryGraphMarket proves composed categories and the resident category
graph appear on FastPump and SpoofLiquidity tapes through the production boot
path — not from cognition buy/sell labels.
*/
func TestCategoryGraphMarket(t *testing.T) {
	proofs := []struct {
		name  string
		state tests.MarketState
	}{
		{"fast pump", tests.MarketStateFastPump},
		{"spoof", tests.MarketStateSpoofLiquidity},
		{"volume absorption", tests.MarketStateVolumeAbsorption},
	}

	Convey("Given warmed production graphs on opportunity and trap tapes", t, func() {
		for _, proof := range proofs {
			Convey("A "+proof.name+" tape publishes composed categories on the graph", func() {
				market := tests.NewMarket(t.Context(), 3)
				wired, err := stack.NewBooter(t.Context()).Test(market)
				So(err, ShouldBeNil)
				Reset(func() {
					So(wired.Close(), ShouldBeNil)
					market.Close()
				})

				So(market.Warmup(tests.Idle), ShouldBeNil)

				var (
					categoryRows int
					withEvidence int
					graphHits    int
					edgeHits     int
				)

				So(market.Transition(proof.state, func() error {
					thesis := wired.Thesis

					if thesis == nil {
						return nil
					}

					categoryRows += len(thesis.Categories)

					for symbol, rows := range thesis.Categories {
						So(symbol, ShouldBeIn, market.Symbols)

						for _, row := range rows {
							So(row.Symbol, ShouldEqual, symbol)
							So(row.Type, ShouldNotEqual, types.CategoryTypeNone)
							So(string(row.Type), ShouldNotBeIn, []string{"buy", "sell", "balanced"})
							So(row.Strength, ShouldBeGreaterThan, 0)

							if len(row.Supporting) > 0 || len(row.Opposing) > 0 {
								withEvidence++
							}
						}
					}

					value, ok := thesis.Graphs.Load("categories")

					if !ok {
						return nil
					}

					graph, ok := value.(*category.Graph)

					if !ok || graph == nil {
						return nil
					}

					graphHits++

					for symbol := range thesis.Categories {
						if graph.Prior(symbol) != types.CategoryTypeNone {
							edgeHits++
							break
						}
					}

					return nil
				}), ShouldBeNil)

				So(categoryRows, ShouldBeGreaterThan, 0)
				So(withEvidence, ShouldBeGreaterThan, 0)
				So(graphHits, ShouldBeGreaterThan, 0)
				So(edgeHits, ShouldBeGreaterThan, 0)
			})
		}
	})
}
