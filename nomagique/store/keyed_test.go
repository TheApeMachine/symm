package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestKeyedNext(t *testing.T) {
	Convey("Keyed holds the latest value per symbol", t, func() {
		memory := store.NewKeyed[float64]()
		eth := store.Slot[float64]{Key: "ETH/USD", Value: 100}
		btc := store.Slot[float64]{Key: "BTC/USD", Value: 200}

		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&eth)) {
				return
			}

			yield(unsafe.Pointer(&btc))
		}

		out := tests.CollectSeq[store.Slot[float64]](memory.Next(in))
		So(len(out), ShouldEqual, 2)
		So(out[0].Key, ShouldEqual, "ETH/USD")
		So(out[0].Value, ShouldEqual, 100.0)
		So(out[1].Key, ShouldEqual, "BTC/USD")
		So(out[1].Value, ShouldEqual, 200.0)

		held := tests.CollectSeq[store.Slot[float64]](memory.Next(nil))
		So(len(held), ShouldEqual, 2)
		So(held[0].Value, ShouldEqual, 100.0)
		So(held[1].Value, ShouldEqual, 200.0)

		eth.Value = 101
		for range memory.Next(func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&eth))
		}) {
		}

		again := tests.CollectSeq[store.Slot[float64]](memory.Next(nil))
		So(len(again), ShouldEqual, 2)
		So(again[0].Value, ShouldEqual, 101.0)
		So(again[1].Value, ShouldEqual, 200.0)
	})
}
