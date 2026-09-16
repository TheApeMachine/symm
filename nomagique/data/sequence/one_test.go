package sequence_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data/sequence"
)

func TestOneNext(t *testing.T) {
	Convey("One borrows the same address across successive runs", t, func() {
		value := 3.0
		one := sequence.NewOne(unsafe.Pointer(&value))
		for _, next := range []float64{3, -2, 7} {
			value = next
			count := 0
			for output := range one.Next(nil) {
				So(output, ShouldEqual, unsafe.Pointer(&value))
				So(*(*float64)(output), ShouldEqual, next)
				count++
			}
			So(count, ShouldEqual, 1)
		}
		So(one.Error(), ShouldBeNil)
	})
}

func BenchmarkOneNext(b *testing.B) {
	value := 1.0
	one := sequence.NewOne(unsafe.Pointer(&value))
	b.ReportAllocs()
	for b.Loop() {
		for range one.Next(nil) {
		}
	}
}
