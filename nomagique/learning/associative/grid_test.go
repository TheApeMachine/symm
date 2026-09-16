package associative_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/tests/market"
)

func TestGridNext(t *testing.T) {
	Convey("The composed Grid accepts raw maps and yields resident owner state", t, func() {
		key := [2]string{"trade", "price"}
		owner := store.NewGet[[2]string, float64](key)
		var metric *float64
		for output := range owner.Next(sequence.NewValue(map[[2]string]float64{key: 100})) {
			metric = (*float64)(output)
		}
		grid := associative.NewGrid[float64](store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "price", Operation: owner, Interests: [][2]string{key}, Metrics: map[string]*float64{"last": metric}})))
		address := [2]string{"price", "last"}

		Convey("Every input completes in order across repeated runs", func() {
			var resident *store.Grid[float64]
			for _, frame := range market.ImpulseTape("BTC/USD", 2) {
				value := frame.Peers[0].Metrics["value"].Raw
				count := 0
				for output := range grid.Next(sequence.NewValue(map[[2]string]float64{key: value})) {
					count++
					next := (*store.Grid[float64])(output)
					So(next.Values[address], ShouldEqual, metric)
					So(*next.Values[address], ShouldEqual, value)
					if resident != nil {
						So(next, ShouldEqual, resident)
					}
					resident = next
				}
				So(count, ShouldEqual, 1)
				So(grid.Error(), ShouldBeNil)
			}
		})

		Convey("Multiple inputs in one run are all forwarded", func() {
			count := 0
			for output := range grid.Next(sequence.NewValue(map[[2]string]float64{key: 101}, map[[2]string]float64{key: 102})) {
				count++
				So(*(*store.Grid[float64])(output).Values[address], ShouldEqual, 100+count)
			}
			So(count, ShouldEqual, 2)
			So(grid.Error(), ShouldBeNil)
		})

		Convey("Stopping and resuming does not rerun registration", func() {
			for range grid.Next(sequence.NewValue(map[[2]string]float64{key: 101}, map[[2]string]float64{key: 102})) {
				break
			}
			So(*metric, ShouldEqual, 101)
			for range grid.Next(sequence.NewValue(map[[2]string]float64{key: 103})) {
			}
			So(*metric, ShouldEqual, 103)
			So(grid.Error(), ShouldBeNil)
		})

		Convey("Invalid registration reaches the composed primitive's error state", func() {
			grid = associative.NewGrid[float64](store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "invalid", Operation: owner, Interests: [][2]string{key}, Metrics: map[string]*float64{"last": nil}})))
			for range grid.Next(sequence.NewValue(map[[2]string]float64{key: 101})) {
				t.Fatal("invalid registration published a grid")
			}
			So(errors.Is(grid.Error(), core.ErrShape), ShouldBeTrue)
			So(*metric, ShouldEqual, 100)
		})
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
	grid := associative.NewGrid[float64](store.NewQuery[float64](nil, data.ActionIdentify, sequence.NewValue(&store.Registration[float64]{Owner: "price", Operation: owner, Interests: [][2]string{key}, Metrics: map[string]*float64{"last": metric}})))
	for range grid.Next(sequence.NewValue(raw)) {
	}
	tape := market.ImpulseTape("BTC/USD", 2)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		raw[key] = tape[index%len(tape)].Peers[0].Metrics["value"].Raw
		for range grid.Next(sequence.NewValue(raw)) {
		}
	}
	if err := grid.Error(); err != nil {
		b.Fatal(err)
	}
}
