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
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/signal/quote"
)

func pushFutures(grid *store.Grid[*geometry.Coordinate], data kraken.FuturesTickerData) {
	query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
		nil, core.Execute,
	)

	for range grid.Next(query.Next(quote.NewFutures().Next(sequence.NewValue(data)))) {
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
	Convey("Given a derivatives ticker on the grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewTicker(t.Context(), grid, "PF_ETHUSD")

		Convey("a futures snapshot updates retained derivative price and basis", func() {
			pushFutures(grid, kraken.FuturesTickerData{
				Symbol:       "PF_ETHUSD",
				Last:         decimal.NewFromFloat64(200),
				IndexPrice:   decimal.NewFromFloat64(100),
				MarkPrice:    decimal.NewFromFloat64(100),
				OpenInterest: 10,
				Timestamp:    time.Unix(1, 0),
			})
			last, have := valueAt(collect(grid), 0)
			So(have, ShouldBeTrue)
			So(last, ShouldEqual, 200.0)
			basis, haveBasis := valueAt(collect(grid), 4)
			So(haveBasis, ShouldBeTrue)
			So(basis, ShouldEqual, 1.0)
		})

		Convey("Ticker.Next does not mutate retained values", func() {
			pushFutures(grid, kraken.FuturesTickerData{
				Symbol:       "PF_ETHUSD",
				Last:         decimal.NewFromFloat64(200),
				IndexPrice:   decimal.NewFromFloat64(100),
				OpenInterest: 10,
				Timestamp:    time.Unix(1, 0),
			})
			before, _ := valueAt(collect(grid), 0)
			entity.Next(sequence.NewValue(kraken.FuturesTickerData{
				Symbol: "PF_ETHUSD",
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

		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			out := tests.CollectSeq[core.Input[*geometry.Coordinate, string, float64]](node.Next(nil))
			So(len(out), ShouldEqual, 0)
			So(node.Status(), ShouldEqual, stage)
		}
	})
}
