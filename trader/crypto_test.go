package trader

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/types"
)

/*
TestSnapshotThesisFreezesMutableGraphAndHolding proves the journal snapshot does
not retain live pointers that continue mutating under broker and analyzer work.
*/
func TestSnapshotThesisFreezesMutableGraphAndHolding(t *testing.T) {
	Convey("Given a live thesis with a holding and resident category graph", t, func() {
		crypto := &Crypto{}
		thesis := types.NewThesis()
		holding := &types.Holding{
			Symbol:     "BTC/USD",
			Mark:       decimal.NewFromInt64(10),
			EntryPrice: decimal.NewFromInt64(10),
			Stoploss: &types.Stoploss{
				Symbol: "BTC/USD",
				Mark:   decimal.NewFromInt64(10),
			},
		}
		graph := category.NewGraph()
		node := &category.Node{Symbol: "BTC/USD", Type: types.VerticalIgnition, Strength: 1, At: time.Unix(1, 0)}
		graph.Nodes = append(graph.Nodes, node)
		graph.Priors["BTC/USD"] = types.VerticalIgnition
		thesis.Holdings.Store("BTC/USD", holding)
		thesis.Lifecycle.Store("BTC/USD", types.LifecycleManaging)
		thesis.Graphs.Store("categories", graph)

		snapshot := crypto.snapshotThesis(thesis, "BTC/USD")

		Convey("When the live thesis mutates afterward", func() {
			holding.Mark = decimal.NewFromInt64(25)
			holding.Stoploss.Mark = decimal.NewFromInt64(25)
			graph.Priors["BTC/USD"] = types.Exhaustion
			node.Strength = 5

			Convey("Then the saved snapshot retains the original frozen values", func() {
				stored, ok := snapshot.Holdings.Load("BTC/USD")
				So(ok, ShouldBeTrue)
				frozen := stored.(*types.Holding)
				So(frozen.Mark.Float64(), ShouldEqual, 10)
				So(frozen.Stoploss.Mark.Float64(), ShouldEqual, 10)

				storedGraph, ok := snapshot.Graphs.Load("categories")
				So(ok, ShouldBeTrue)
				frozenGraph := storedGraph.(*category.Graph)
				So(frozenGraph.Priors["BTC/USD"], ShouldEqual, types.VerticalIgnition)
				So(frozenGraph.Nodes[0].Strength, ShouldEqual, 1)
			})
		})
	})
}
