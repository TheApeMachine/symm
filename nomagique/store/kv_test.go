package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestKVNext(t *testing.T) {
	Convey("A fresh KV merges arrivals without mutating the configured source", t, func() {
		seed := map[string]float64{"existing": 7}
		op := store.NewKV[string, float64](seed)

		m1 := map[string]float64{"mean": 10}
		m2 := map[string]float64{"count": 3}
		m3 := map[string]float64{"mean": -5}
		m4 := map[string]float64{"existing": 12}

		in := func(yield func(unsafe.Pointer) bool) {
			for _, m := range []map[string]float64{m1, m2, m3, m4} {
				if !yield(unsafe.Pointer(&m)) {
					return
				}
			}
		}

		first := tests.CollectSeq[map[string]float64](op.Next(in))

		So(first[len(first)-1]["mean"], ShouldEqual, -5)
		So(first[len(first)-1]["count"], ShouldEqual, 3)
		So(first[len(first)-1]["existing"], ShouldEqual, 12)
		So(seed["existing"], ShouldEqual, 7)
	})
}
