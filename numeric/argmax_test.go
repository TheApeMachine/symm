package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArgmaxStringFloat64(t *testing.T) {
	Convey("ArgmaxStringFloat64", t, func() {
		So(func() { ArgmaxStringFloat64(nil) }, ShouldNotPanic)

		emptyKey, emptyVal := ArgmaxStringFloat64(nil)
		So(emptyKey, ShouldEqual, "")
		So(emptyVal, ShouldEqual, -1)

		k, v := ArgmaxStringFloat64(map[string]float64{"a": 1, "b": 3, "c": 2})
		So(k, ShouldEqual, "b")
		So(v, ShouldEqual, 3)
	})
}

func BenchmarkArgmaxStringFloat64(b *testing.B) {
	m := make(map[string]float64, 32)

	for i := range 32 {
		m[string(rune('a'+i))] = float64(i)
	}

	b.ResetTimer()

	for b.Loop() {
		_, _ = ArgmaxStringFloat64(m)
	}
}
