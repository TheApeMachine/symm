package pumpdump

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

func pushTrade(grid *store.Grid[*geometry.Coordinate], data kraken.TradeData) {
	query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
		nil, core.Execute,
	)

	for range grid.Next(query.Next(quote.NewTrade().Next(sequence.NewValue(data)))) {
	}
}

func TestTradeNext(t *testing.T) {
	Convey("Given a pumpdump trade instrument on the grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewTrade(t.Context(), grid, "BTC/USD")

		Convey("an execution updates retained price and quantity", func() {
			pushTrade(grid, kraken.TradeData{
				Symbol:    "BTC/USD",
				Price:     *decimal.NewFromFloat64(100),
				Qty:       2,
				Timestamp: time.Unix(1, 0),
			})
			readings := collect(grid)
			price, havePrice := valueAt(readings, 0)
			So(havePrice, ShouldBeTrue)
			So(price, ShouldEqual, 100.0)
			qty, haveQty := valueAt(readings, 2)
			So(haveQty, ShouldBeTrue)
			So(qty, ShouldEqual, 2.0)
		})

		Convey("Trade.Next does not mutate retained values", func() {
			pushTrade(grid, kraken.TradeData{
				Symbol:    "BTC/USD",
				Price:     *decimal.NewFromFloat64(100),
				Qty:       2,
				Timestamp: time.Unix(1, 0),
			})
			before, _ := valueAt(collect(grid), 0)
			entity.Next(sequence.NewValue(kraken.TradeData{
				Symbol: "BTC/USD",
				Price:  *decimal.NewFromFloat64(1),
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
