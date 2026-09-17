package derivatives

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/signal/quote"
)

func pushFuturesTrade(grid *store.Grid[*geometry.Coordinate], data kraken.FuturesTradeData) {
	query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
		nil, core.Execute,
	)

	for range grid.Next(query.Next(quote.NewFuturesTrade().Next(sequence.NewValue(data)))) {
	}
}

func TestTradeNext(t *testing.T) {
	Convey("Given a derivatives trade instrument on the grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewTrade(t.Context(), grid, "PF_ETHUSD")

		Convey("an execution updates retained gross notional", func() {
			pushFuturesTrade(grid, kraken.FuturesTradeData{
				Symbol:    "PF_ETHUSD",
				Side:      "buy",
				Price:     *decimal.NewFromFloat64(10),
				Qty:       5,
				Timestamp: time.Unix(1, 0),
			})
			notional, have := valueAt(collect(grid), 0)
			So(have, ShouldBeTrue)
			So(notional, ShouldEqual, 50.0)
		})

		Convey("Trade.Next does not mutate retained values", func() {
			pushFuturesTrade(grid, kraken.FuturesTradeData{
				Symbol:    "PF_ETHUSD",
				Side:      "buy",
				Price:     *decimal.NewFromFloat64(10),
				Qty:       5,
				Timestamp: time.Unix(1, 0),
			})
			before, _ := valueAt(collect(grid), 0)
			entity.Next(sequence.NewValue(kraken.FuturesTradeData{
				Symbol: "PF_ETHUSD",
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
