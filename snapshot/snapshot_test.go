package snapshot

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type cellState struct {
	last  float64
	bid   float64
	ask   float64
	ticks []float64
}

func TestCellLoadMutate(t *testing.T) {
	Convey("Given a fresh Cell", t, func() {
		cell := New(cellState{last: 1, bid: 2, ask: 3})

		Convey("Load returns the stored snapshot", func() {
			snap, ok := cell.Load()
			So(ok, ShouldBeTrue)
			So(snap.last, ShouldEqual, 1)
			So(snap.bid, ShouldEqual, 2)
			So(snap.ask, ShouldEqual, 3)
		})

		Convey("Mutate composes with concurrent writers", func() {
			cell := New(cellState{})

			var wg sync.WaitGroup

			for i := 0; i < 64; i++ {
				wg.Add(1)
				value := float64(i + 1)

				go func() {
					defer wg.Done()
					cell.Mutate(func(state *cellState) bool {
						state.ticks = Append(state.ticks, value, 0)
						return true
					})
				}()
			}

			wg.Wait()
			snap, _ := cell.Load()
			So(len(snap.ticks), ShouldEqual, 64)
		})

		Convey("Mutate returning false leaves the snapshot untouched", func() {
			cell := New(cellState{last: 7})
			cell.Mutate(func(state *cellState) bool {
				state.last = 99
				return false
			})

			snap, _ := cell.Load()
			So(snap.last, ShouldEqual, 7)
		})
	})
}

func TestAppendAllocatesNewBackingOnGrowth(t *testing.T) {
	Convey("Given a small append-only slice", t, func() {
		base := []int{1, 2, 3}
		next := Append(base, 4, 0)

		Convey("The original backing array is not shared with the new slice", func() {
			next[0] = 999
			So(base[0], ShouldEqual, 1)
		})

		Convey("Append with cap trims from the front", func() {
			out := []int{1, 2, 3}
			out = Append(out, 4, 3)
			out = Append(out, 5, 3)
			So(out, ShouldResemble, []int{3, 4, 5})
		})
	})
}
