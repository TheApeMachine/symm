package correlation

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
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func timestamp(second int64) time.Time {
	return time.Unix(1_700_000_000+second, 0)
}

func tickerData(symbol string, last float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      decimal.NewFromFloat64(last),
		Bid:       decimal.NewFromFloat64(last - 0.5),
		Ask:       decimal.NewFromFloat64(last + 0.5),
		BidQty:    3,
		AskQty:    4,
		Volume:    1000,
		Timestamp: at,
	}
}

func push(grid *store.Grid[*geometry.Coordinate], data kraken.TickerData) {
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
	Convey("Given an explicit pair registered on the grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewTicker(t.Context(), grid, "ETH/USD", "BTC/USD")

		Convey("each metric registers as a grid cell before any observation", func() {
			So(len(collect(grid)), ShouldEqual, 0)

			for index := range 25 {
				address := transport.NewAddress[*geometry.Coordinate]()
				address.Identify(geometry.NewCoordinate(index, 0))
				query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
					address, core.Read,
				)
				for range grid.Next(query.Next(nil)) {
				}
				So(grid.Error(), ShouldBeNil)
			}

			missing := transport.NewAddress[*geometry.Coordinate]()
			missing.Identify(geometry.NewCoordinate(25, 0))
			query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				missing, core.Read,
			)
			for range grid.Next(query.Next(nil)) {
			}
			So(grid.Error(), ShouldNotBeNil)
		})

		Convey("raw ticker data updates last price through declared interests", func() {
			push(grid, tickerData("ETH/USD", 100, timestamp(1)))
			readings := collect(grid)
			last, haveLast := valueAt(readings, 0)
			So(haveLast, ShouldBeTrue)
			So(last, ShouldEqual, 100.0)
			_, haveSigned := valueAt(readings, 1)
			So(haveSigned, ShouldBeFalse)
		})

		Convey("an undeclared symbol does not share pair state", func() {
			push(grid, tickerData("ETH/USD", 100, timestamp(1)))
			push(grid, tickerData("SOL/USD", 50, timestamp(1)))
			push(grid, tickerData("BTC/USD", 200, timestamp(1)))
			readings := collect(grid)
			_, haveSigned := valueAt(readings, 1)
			So(haveSigned, ShouldBeFalse)
		})

		Convey("an explicit pair produces Hayashi-Yoshida correlation", func() {
			for index := range 5 {
				push(grid, tickerData("BTC/USD", 100+float64(index), timestamp(int64(index)+1)))
			}

			for index := range 5 {
				push(grid, tickerData("ETH/USD", 200+float64(index)*2, timestamp(int64(index)+1)))
			}

			readings := collect(grid)
			last, haveLast := valueAt(readings, 0)
			So(haveLast, ShouldBeTrue)
			So(last, ShouldEqual, 208.0)
			count, haveCount := valueAt(readings, 2)
			So(haveCount, ShouldBeTrue)
			So(count, ShouldEqual, 5.0)
			signed, haveSigned := valueAt(readings, 1)
			So(haveSigned, ShouldBeTrue)
			So(signed > 0 && signed <= 1, ShouldBeTrue)
			absolute, _ := valueAt(readings, 3)
			So(absolute, ShouldEqual, signed)
			overlap, haveOverlap := valueAt(readings, 12)
			So(haveOverlap, ShouldBeTrue)
			So(overlap, ShouldEqual, 4.0)
		})

		Convey("repeated grid reads do not recompute or mutate retained values", func() {
			for index := range 5 {
				push(grid, tickerData("BTC/USD", 100+float64(index), timestamp(int64(index)+1)))
				push(grid, tickerData("ETH/USD", 200+float64(index)*2, timestamp(int64(index)+1)))
			}

			first := collect(grid)
			second := collect(grid)
			signedFirst, _ := valueAt(first, 1)
			signedSecond, _ := valueAt(second, 1)
			So(signedSecond, ShouldEqual, signedFirst)
			So(len(second), ShouldEqual, len(first))
		})

		Convey("Ticker.Next only exports and does not mutate metric state", func() {
			push(grid, tickerData("ETH/USD", 100, timestamp(1)))
			before := collect(grid)
			exported := tests.CollectSeq[core.Input[*geometry.Coordinate, string, float64]](
				entity.Next(sequence.NewValue(tickerData("ETH/USD", 999, timestamp(2)))),
			)
			after := collect(grid)
			lastBefore, _ := valueAt(before, 0)
			lastAfter, _ := valueAt(after, 0)
			So(lastAfter, ShouldEqual, lastBefore)
			So(lastAfter, ShouldEqual, 100.0)
			So(len(exported), ShouldEqual, len(after))
		})

		Convey("disjoint timestamps leave correlation undefined", func() {
			push(grid, tickerData("BTC/USD", 100, timestamp(1)))
			push(grid, tickerData("BTC/USD", 101, timestamp(2)))
			push(grid, tickerData("ETH/USD", 200, timestamp(100)))
			push(grid, tickerData("ETH/USD", 202, timestamp(101)))
			readings := collect(grid)
			_, haveSigned := valueAt(readings, 1)
			So(haveSigned, ShouldBeFalse)
		})

		Convey("no cohort metrics exist without an explicit cohort", func() {
			readings := collect(grid)
			So(len(readings), ShouldEqual, 0)
			push(grid, tickerData("ETH/USD", 100, timestamp(1)))
			after := collect(grid)
			So(len(after), ShouldBeLessThan, 31)
		})

		Convey("the association pipeline consumes coordinate-labelled observations", func() {
			push(grid, tickerData("ETH/USD", 100, timestamp(1)))
			readings := collect(grid)
			So(len(readings), ShouldBeGreaterThan, 0)
			So(readings[0].Origin.Identity().X, ShouldEqual, 0)

			sympathy := statistic.NewSympathy[*geometry.Coordinate]()
			obs := statistic.NewObservation(readings[0].Origin.Identity(), *readings[0].Value, 1.0, 1.0)

			for range sympathy.Next(sequence.NewValue(*obs)) {
			}

			So(sympathy.Error(), ShouldBeNil)
		})
	})
}

func TestTickerStepReadiness(t *testing.T) {
	Convey("An inactive ticker does not read the grid", t, func() {
		node := &Ticker{System: runtime.NewSystem(t.Context(), "readiness-test")}

		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			out := tests.CollectSeq[core.Input[*geometry.Coordinate, string, float64]](
				node.Next(sequence.NewValue(tickerData("BTC/USD", 100, timestamp(7)))),
			)
			So(len(out), ShouldEqual, 0)
			So(node.Status(), ShouldEqual, stage)
		}
	})
}
