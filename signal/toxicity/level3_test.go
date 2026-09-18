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
	"github.com/theapemachine/symm/nomagique/transport"
)

func pushTouch(grid *store.Grid[*geometry.Coordinate], data kraken.Level3Touch) {
	query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
		nil, core.Execute,
	)
	pipeline := nomagique.NewNumber(query, grid)

	for range pipeline.Next(sequence.NewValue(map[string]any{
		"level3": map[string]any{
			"data": map[string]any{
				"symbol":    data.Symbol,
				"bid":       data.Bid.Float64(),
				"ask":       data.Ask.Float64(),
				"bid_qty":   data.BidQty,
				"ask_qty":   data.AskQty,
				"timestamp": data.Timestamp.UnixNano(),
			},
		},
	})) {
	}
}

func collect(grid *store.Grid[*geometry.Coordinate]) []core.Input[*geometry.Coordinate, string, float64] {
	address := transport.NewAddress[*geometry.Coordinate]()
	query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
		address, core.Read,
	)
	return tests.CollectSeq[core.Input[*geometry.Coordinate, string, float64]](grid.Next(query.Next(nil)))
}

func valueAt(readings []core.Input[*geometry.Coordinate, string, float64], x int) (float64, bool) {
	for _, reading := range readings {
		if reading.Origin == nil || reading.Value == nil {
			continue
		}

		if reading.Origin.Identity().X == x {
			return *reading.Value, true
		}
	}

	return 0, false
}

func TestLevel3Next(t *testing.T) {
	Convey("Given a toxicity level3 instrument on the grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewLevel3(t.Context(), grid, "ETH/USD")

		Convey("a two-sided touch updates retained bid", func() {
			pushTouch(grid, kraken.Level3Touch{
				Symbol:    "ETH/USD",
				Bid:       decimal.NewFromFloat64(50),
				Ask:       decimal.NewFromFloat64(51),
				BidQty:    decimal.NewFromFloat64(1),
				AskQty:    decimal.NewFromFloat64(2),
				Timestamp: time.Unix(1, 0),
			})
			bid, have := valueAt(collect(grid), 0)
			So(have, ShouldBeTrue)
			So(bid, ShouldEqual, 50.0)
		})

		Convey("Level3.Next does not mutate retained values", func() {
			pushTouch(grid, kraken.Level3Touch{
				Symbol:    "ETH/USD",
				Bid:       decimal.NewFromFloat64(50),
				Ask:       decimal.NewFromFloat64(51),
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
