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

func pushTouch(grid *store.Grid[*geometry.Coordinate], data kraken.Level3Touch) {
	query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
		nil, core.Execute,
	)

	for range grid.Next(query.Next(quote.NewTouch().Next(sequence.NewValue(data)))) {
	}
}

func TestLevel3Next(t *testing.T) {
	Convey("Given a pumpdump level3 instrument on the grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewLevel3(t.Context(), grid, "ETH/USD")

		Convey("a two-sided touch updates retained bid and spread", func() {
			pushTouch(grid, kraken.Level3Touch{
				Symbol:    "ETH/USD",
				Bid:       decimal.NewFromFloat64(99),
				Ask:       decimal.NewFromFloat64(101),
				Timestamp: time.Unix(1, 0),
			})
			readings := collect(grid)
			bid, haveBid := valueAt(readings, 0)
			So(haveBid, ShouldBeTrue)
			So(bid, ShouldEqual, 99.0)
			spread, haveSpread := valueAt(readings, 4)
			So(haveSpread, ShouldBeTrue)
			So(spread, ShouldEqual, 2.0)
		})

		Convey("Level3.Next does not mutate retained values", func() {
			pushTouch(grid, kraken.Level3Touch{
				Symbol:    "ETH/USD",
				Bid:       decimal.NewFromFloat64(99),
				Ask:       decimal.NewFromFloat64(101),
				Timestamp: time.Unix(1, 0),
			})
			before, _ := valueAt(collect(grid), 0)
			entity.Next(sequence.NewValue(kraken.Level3Touch{
				Symbol: "ETH/USD",
				Bid:    decimal.NewFromFloat64(1),
				Ask:    decimal.NewFromFloat64(2),
			}))
			after, _ := valueAt(collect(grid), 0)
			So(after, ShouldEqual, before)
		})
	})
}

func TestLevel3StepReadiness(t *testing.T) {
	Convey("An inactive level3 does not read the grid", t, func() {
		node := &Level3{System: runtime.NewSystem(t.Context(), "readiness-test")}

		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			out := tests.CollectSeq[core.Input[*geometry.Coordinate, string, float64]](node.Next(nil))
			So(len(out), ShouldEqual, 0)
			So(node.Status(), ShouldEqual, stage)
		}
	})
}
