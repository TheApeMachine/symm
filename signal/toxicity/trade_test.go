package toxicity

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func pushTrade(grid *store.Grid[*geometry.Coordinate], data kraken.TradeData) {
	query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
		nil, core.Execute,
	)
	pipeline := nomagique.NewNumber(query, grid)

	for range pipeline.Next(sequence.NewValue(map[string]any{
		"trade": map[string]any{
			"data": map[string]any{
				"symbol":    data.Symbol,
				"side":      data.Side,
				"price":     data.Price.Float64(),
				"qty":       data.Qty,
				"timestamp": data.Timestamp.UnixNano(),
			},
		},
	})) {
	}
}

func TestTradeNext(t *testing.T) {
	Convey("Given a toxicity trade instrument on the grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewTrade(t.Context(), grid, "BTC/USD")

		Convey("an execution updates retained quantity", func() {
			pushTrade(grid, kraken.TradeData{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     *decimal.NewFromFloat64(100),
				Qty:       3,
				Timestamp: time.Unix(1, 0),
			})
			qty, have := valueAt(collect(grid), 0)
			So(have, ShouldBeTrue)
			So(qty, ShouldEqual, 3.0)
		})

		Convey("Trade.Next does not mutate retained values", func() {
			pushTrade(grid, kraken.TradeData{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     *decimal.NewFromFloat64(100),
				Qty:       3,
				Timestamp: time.Unix(1, 0),
			})
			before, _ := valueAt(collect(grid), 0)
			entity.Next(sequence.NewValue(kraken.TradeData{
				Symbol: "BTC/USD",
				Qty:    9,
			}))
			after, _ := valueAt(collect(grid), 0)
			So(after, ShouldEqual, before)
		})
	})
}

func TestTradeStepReadiness(t *testing.T) {
	Convey("An inactive trade does not read the grid", t, func() {
		node := &Trade{System: runtime.NewSystem(t.Context(), "readiness-test")}

		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			out := tests.CollectSeq[core.Input[*geometry.Coordinate, string, float64]](node.Next(nil))
			So(len(out), ShouldEqual, 0)
			So(node.Status(), ShouldEqual, stage)
		}
	})
}
