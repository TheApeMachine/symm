package snapshot

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAppendWithPool(t *testing.T) {
	Convey("Given a slice pool", t, func() {
		slicePool := NewSlicePool[int](4)

		Convey("When appending past the soft cap", func() {
			source := slicePool.Acquire()
			source = append(source, 1, 2, 3)
			next := AppendWithPool(slicePool, source, 4, 3)

			Convey("It should retain only the latest cap rows", func() {
				So(next, ShouldResemble, []int{2, 3, 4})
			})
		})
	})
}

func BenchmarkAppendWithPool(b *testing.B) {
	slicePool := NewSlicePool[int](1024)
	source := slicePool.Acquire()

	for index := range 512 {
		source = append(source, index)
	}

	b.ReportAllocs()

	for b.Loop() {
		next := AppendWithPool(slicePool, source, 999, 1024)
		slicePool.Release(next)
	}
}

func BenchmarkAppend(b *testing.B) {
	source := make([]int, 0, 1024)

	for index := range 512 {
		source = append(source, index)
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = Append(source, 999, 1024)
	}
}
