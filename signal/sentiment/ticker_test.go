package sentiment

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

func pushTicker(grid *store.Grid[*geometry.Coordinate], data kraken.TickerData) {
	query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
		nil, core.Execute,
	)
	pipeline := nomagique.NewNumber(query, grid)

	for range pipeline.Next(sequence.NewValue(map[string]any{
		"ticker": map[string]any{
			"data": map[string]any{
				"symbol":    data.Symbol,
				"last":      data.Last.Float64(),
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

func TestTickerNext(t *testing.T) {
	Convey("Given a sentiment instrument on the grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewTicker(t.Context(), grid, "ETH/USD")

		Convey("a last price is retained", func() {
			pushTicker(grid, kraken.TickerData{
				Symbol:    "ETH/USD",
				Last:      decimal.NewFromFloat64(100),
				Timestamp: time.Unix(1, 0),
			})
			last, have := valueAt(collect(grid), 0)
			So(have, ShouldBeTrue)
			So(last, ShouldEqual, 100.0)
		})

		Convey("Ticker.Next does not mutate retained values", func() {
			pushTicker(grid, kraken.TickerData{
				Symbol:    "ETH/USD",
				Last:      decimal.NewFromFloat64(100),
				Timestamp: time.Unix(1, 0),
			})
			before, _ := valueAt(collect(grid), 0)
			entity.Next(sequence.NewValue(kraken.TickerData{
				Symbol: "ETH/USD",
				Last:   decimal.NewFromFloat64(1),
			}))
			after, _ := valueAt(collect(grid), 0)
			So(after, ShouldEqual, before)
		})
	})
}

func TestTickerStepReadiness(t *testing.T) {
	Convey("An inactive ticker does not read the grid", t, func() {
		node := &Ticker{System: runtime.NewSystem(t.Context(), "readiness-test")}
		node.Transition(runtime.INIT)
		So(len(tests.CollectSeq[core.Input[*geometry.Coordinate, string, float64]](node.Next(nil))), ShouldEqual, 0)
	})
}
