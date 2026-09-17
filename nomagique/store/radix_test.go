package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestRadixNext(t *testing.T) {
	Convey("An explicit query/store/add/query/store composition owns addressed counts", t, func() {
		radix := store.NewRadix[float64]()
		key := []byte("BTC/USD:enter")
		pipeline := nomagique.NewNumber(
			store.NewKeyQuery[float64](&key, data.ActionIdentify, sequence.NewValues(0.0).Next(nil)), radix, sequence.NewZip2[float64](sequence.NewValues(1.0).Next(nil)), arithmetic.NewAdd(),
			store.NewKeyQuery[float64](&key, data.ActionWrite), radix,
		)
		for _, expected := range []float64{1, 2, 3} {
			values := tests.CollectSeq[float64](pipeline.Next(nil))
			So(pipeline.Error(), ShouldBeNil)
			So(values, ShouldResemble, []float64{expected})
		}
		Convey("A different address has independent evidence", func() {
			key = []byte("ETH/USD:enter")
			So(tests.CollectSeq[float64](pipeline.Next(nil)), ShouldResemble, []float64{1})
			key = []byte("BTC/USD:enter")
			So(tests.CollectSeq[float64](pipeline.Next(nil)), ShouldResemble, []float64{4})
		})
		Convey("An unseen read remains absent and does not create evidence", func() {
			key = []byte("missing")
			read := nomagique.NewNumber(store.NewKeyQuery[float64](&key, data.ActionRead), radix)
			So(tests.CollectSeq[float64](read.Next(nil)), ShouldBeEmpty)
			So(read.Error(), ShouldBeNil)
		})
		Convey("Reusing the caller's address buffer cannot mutate published keys", func() {
			copy(key, []byte("ETH/USD:enter"))
			So(tests.CollectSeq[float64](pipeline.Next(nil)), ShouldResemble, []float64{1})
			copy(key, []byte("BTC/USD:enter"))
			So(tests.CollectSeq[float64](pipeline.Next(nil)), ShouldResemble, []float64{4})
		})
		Convey("Changing the caller's write value does not mutate stored evidence", func() {
			value := 7.0
			write := nomagique.NewNumber(sequence.NewOne(unsafe.Pointer(&value)), store.NewKeyQuery[float64](&key, data.ActionWrite), radix)
			So(tests.CollectSeq[float64](write.Next(nil)), ShouldResemble, []float64{7})
			value = 99
			read := nomagique.NewNumber(store.NewKeyQuery[float64](&key, data.ActionRead), radix)
			So(tests.CollectSeq[float64](read.Next(nil)), ShouldResemble, []float64{7})
		})
	})
}

func BenchmarkRadixNext(b *testing.B) {
	radix := store.NewRadix[float64]()
	key := []byte("BTC/USD:enter")
	pipeline := nomagique.NewNumber(
		store.NewKeyQuery[float64](&key, data.ActionIdentify, sequence.NewValues(0.0).Next(nil)), radix, sequence.NewZip2[float64](sequence.NewValues(1.0).Next(nil)), arithmetic.NewAdd(),
		store.NewKeyQuery[float64](&key, data.ActionWrite), radix,
	)
	b.ReportAllocs()
	for b.Loop() {
		for range pipeline.Next(nil) {
		}
	}
	if err := pipeline.Error(); err != nil {
		b.Fatal(err)
	}
}
