package store_test

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/tests/market"
)

func TestGridNext(t *testing.T) {
	Convey("A Grid routes raw input to the sole writer of resident metrics", t, func() {
		priceKey := [2]string{"trade", "price"}
		quantityKey := [2]string{"trade", "quantity"}
		owner := nomagique.NewNumber(store.NewGet[[2]string, float64](priceKey), statistic.NewEstimator())
		// This observation establishes the real estimator's resident reading.
		var reading *statistic.MomentReading
		for output := range owner.Next(sequence.NewValue(map[[2]string]float64{priceKey: 100})) {
			reading = (*statistic.MomentReading)(output)
		}
		metrics := map[string]*float64{"price": &reading.Value, "support": &reading.Count}
		grid := store.NewGrid[float64]()
		registration := store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "signal", Operation: owner, Interests: [][2]string{priceKey, quantityKey}, Metrics: metrics}))
		for output := range grid.Next(registration.Next(nil)) {
			So((*store.Grid[float64])(output), ShouldEqual, grid)
		}
		pipeline := nomagique.NewNumber(store.NewQuery[float64](nil, data.ActionExecute), grid)

		Convey("Registration only binds references and canonical coordinates", func() {
			So(reading.Count, ShouldEqual, 1)
			So(grid.Values[[2]string{"signal", "price"}], ShouldEqual, &reading.Value)
			So(*grid.Coordinates[[2]string{"signal", "price"}], ShouldResemble, [2]float64{0, 0})
			So(*grid.Coordinates[[2]string{"signal", "support"}], ShouldResemble, [2]float64{1, 0})
		})

		Convey("Multi-leg raw inputs execute the owner once, even when both interests match", func() {
			tape := market.ImpulseTape("BTC/USD", 2)
			position := grid.Coordinates[[2]string{"signal", "price"}]
			*position = [2]float64{5, -3} // A coordinate move, not a value relocation.
			for index, frame := range tape {
				price := frame.Peers[0].Metrics["value"].Raw
				raw := map[[2]string]float64{priceKey: price, quantityKey: 1}
				outputs := 0
				for output := range pipeline.Next(sequence.NewValue(raw)) {
					outputs++
					So((*store.Grid[float64])(output), ShouldEqual, grid)
				}
				So(outputs, ShouldEqual, 1)
				So(reading.Count, ShouldEqual, index+2)
				So(*grid.Values[[2]string{"signal", "price"}], ShouldEqual, price)
				So(grid.Values[[2]string{"signal", "price"}], ShouldEqual, &reading.Value)
				So(grid.Coordinates[[2]string{"signal", "price"}], ShouldEqual, position)
				So(*position, ShouldResemble, [2]float64{5, -3})
				So(raw, ShouldResemble, map[[2]string]float64{priceKey: price, quantityKey: 1})
			}
			So(pipeline.Error(), ShouldBeNil)
		})

		Convey("Entity and key must both match; unrelated input does not invoke the owner", func() {
			for range pipeline.Next(sequence.NewValue(map[[2]string]float64{{"ticker", "price"}: 200, {"trade", "price_extra"}: 300})) {
			}
			So(reading.Count, ShouldEqual, 1)
			So(pipeline.Error(), ShouldBeNil)
		})

		Convey("Reading the index does not run or mutate the owner", func() {
			for output := range grid.Next(store.NewQuery[float64](nil, data.ActionRead).Next(nil)) {
				So((*store.Grid[float64])(output), ShouldEqual, grid)
			}
			So(reading.Count, ShouldEqual, 1)
		})

		Convey("A second owner cannot claim an existing metric's storage", func() {
			other := store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "other", Operation: owner, Interests: [][2]string{priceKey}, Metrics: metrics}))
			for range grid.Next(other.Next(nil)) {
				t.Fatal("invalid registration produced an answer")
			}
			So(errors.Is(grid.Error(), core.ErrShape), ShouldBeTrue)
			So(len(grid.Values), ShouldEqual, 2)
		})

		Convey("An owner cannot alias two metric identities onto the same storage", func() {
			independent := store.NewGrid[float64]()
			aliased := store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "alias", Operation: owner, Interests: [][2]string{priceKey}, Metrics: map[string]*float64{"first": &reading.Value, "second": &reading.Value}}))
			for range independent.Next(aliased.Next(nil)) {
				t.Fatal("aliased registration produced an answer")
			}
			So(errors.Is(independent.Error(), core.ErrShape), ShouldBeTrue)
			So(len(independent.Values), ShouldEqual, 0)
		})

		Convey("An owner error prevents publication of a completed boundary", func() {
			// Quantity selects this owner, but its required price is absent.
			for range pipeline.Next(sequence.NewValue(map[[2]string]float64{quantityKey: 1})) {
				t.Fatal("failed boundary was published")
			}
			So(errors.Is(grid.Error(), core.ErrNotHeld), ShouldBeTrue)
			So(reading.Count, ShouldEqual, 1)
		})

		Convey("Stopping consumption stops before the next raw observation", func() {
			for range pipeline.Next(sequence.NewValue(map[[2]string]float64{priceKey: 101}, map[[2]string]float64{priceKey: 102})) {
				break
			}
			So(reading.Count, ShouldEqual, 2)
			So(reading.Value, ShouldEqual, 101)
			So(pipeline.Error(), ShouldBeNil)
		})

		Convey("The store rejects direct writes to owner-held metrics", func() {
			for range grid.Next(store.NewQuery[float64](nil, data.ActionWrite, sequence.NewValue(42.0)).Next(nil)) {
				t.Fatal("direct metric write was accepted")
			}
			So(errors.Is(grid.Error(), core.ErrShape), ShouldBeTrue)
			So(reading.Value, ShouldEqual, 100)
		})
	})
	Convey("Registration order cannot change owner execution or coordinate identity", t, func() {
		firstKey, secondKey := [2]string{"trade", "first"}, [2]string{"trade", "second"}
		first := store.NewGet[[2]string, float64](firstKey)
		var firstMetric, secondMetric *float64
		for output := range first.Next(sequence.NewValue(map[[2]string]float64{firstKey: 100})) {
			firstMetric = (*float64)(output)
		}
		second := nomagique.NewNumber(
			store.NewGet[[2]string, float64](secondKey), sequence.
				NewZip2[float64](sequence.NewOne(unsafe.Pointer(firstMetric)).Next(nil)), arithmetic.NewAdd(),
		)
		for output := range second.Next(sequence.NewValue(map[[2]string]float64{secondKey: 20})) {
			secondMetric = (*float64)(output)
		}
		registrations := []*store.Query[float64]{store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "a", Operation: first, Interests: [][2]string{firstKey}, Metrics: map[string]*float64{"value": firstMetric}})), store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "b", Operation: second, Interests: [][2]string{secondKey}, Metrics: map[string]*float64{"value": secondMetric}}))}
		for _, order := range [][2]int{{0, 1}, {1, 0}} {
			grid := store.NewGrid[float64]()
			for _, index := range order {
				for range grid.Next(registrations[index].Next(nil)) {
				}
			}
			pipeline := nomagique.NewNumber(store.NewQuery[float64](nil, data.ActionExecute), grid)
			for range pipeline.Next(sequence.NewValue(map[[2]string]float64{firstKey: 10, secondKey: 3})) {
			}
			So(*firstMetric, ShouldEqual, 10)
			So(*secondMetric, ShouldEqual, 13)
			So(*grid.Coordinates[[2]string{"a", "value"}], ShouldResemble, [2]float64{0, 0})
			So(*grid.Coordinates[[2]string{"b", "value"}], ShouldResemble, [2]float64{1, 0})
			So(pipeline.Error(), ShouldBeNil)
			// The owner restores its prior value before the reverse ordering.
			for range first.Next(sequence.NewValue(map[[2]string]float64{firstKey: 100})) {
			}
		}
	})

}

func BenchmarkGridNext(b *testing.B) {
	key := [2]string{"trade", "price"}
	owner := store.NewGet[[2]string, float64](key)
	raw := map[[2]string]float64{key: 100}
	var metric *float64
	for output := range owner.Next(sequence.NewValue(raw)) {
		metric = (*float64)(output)
	}
	grid := store.NewGrid[float64]()
	for range grid.Next(store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "signal", Operation: owner, Interests: [][2]string{key}, Metrics: map[string]*float64{"price": metric}})).Next(nil)) {
	}
	pipeline := nomagique.NewNumber(store.NewQuery[float64](nil, data.ActionExecute), grid)
	tape := market.ImpulseTape("BTC/USD", 2)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		raw[key] = tape[index%len(tape)].Peers[0].Metrics["value"].Raw
		for range pipeline.Next(sequence.NewValue(raw)) {
		}
	}
	if err := pipeline.Error(); err != nil {
		b.Fatal(err)
	}
}
