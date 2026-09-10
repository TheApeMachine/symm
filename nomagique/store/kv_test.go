package store

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestKVNext(t *testing.T) {
	Convey("A fresh KV merges arrivals without mutating the configured source", t, func() {
		seed := map[string]float64{"existing": 7}
		op := NewKV[string, float64](seed)

		first := tests.CollectSeq(op.Next(transport.Values(
			map[string]float64{"mean": 10},
			map[string]float64{"count": 3},
			map[string]float64{"mean": -5},
			map[string]float64{"existing": 12},
		)))

		So(first[len(first)-1]["mean"], ShouldEqual, -5)
		So(first[len(first)-1]["count"], ShouldEqual, 3)
		So(first[len(first)-1]["existing"], ShouldEqual, 12)
		So(seed["existing"], ShouldEqual, 7)
	})
}
