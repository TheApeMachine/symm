package pool

import (
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestPoolNext(t *testing.T) {
	Convey("Given an elastic pool primitive", t, func() {
		Convey("Every task is delivered exactly once and results travel the wire", func() {
			tasks := make([]int, 64)

			for index := range tasks {
				tasks[index] = index + 1
			}

			op := NewPool(func(task *int) { *task *= 2 }, 40*time.Millisecond)
			count := 0

			for range op.Next(tests.SliceToSeq(tasks)) {
				count++
			}

			So(count, ShouldEqual, 64)
			So(op.Error(), ShouldBeNil)

			for index := range tasks {
				So(tasks[index], ShouldEqual, 2*(index+1))
			}
		})

		Convey("A burst of blocked tasks scales workers up", func() {
			var concurrent, maxConcurrent atomic.Int64

			handler := func(task *int) {
				current := concurrent.Add(1)

				for {
					observed := maxConcurrent.Load()

					if observed >= current {
						break
					}

					if maxConcurrent.CompareAndSwap(observed, current) {
						break
					}
				}

				time.Sleep(20 * time.Millisecond)
				concurrent.Add(-1)
			}

			tasks := make([]int, 12)
			op := NewPool(handler, 40*time.Millisecond)
			count := 0

			for range op.Next(tests.SliceToSeq(tasks)) {
				count++
			}

			So(count, ShouldEqual, 12)
			So(maxConcurrent.Load(), ShouldBeGreaterThanOrEqualTo, 2)
			So(op.Error(), ShouldBeNil)
		})

		Convey("The stream drains every queued task before ending", func() {
			var executed atomic.Int64
			tasks := make([]int, 128)

			op := NewPool(func(task *int) { executed.Add(1) }, 40*time.Millisecond)
			count := 0

			for range op.Next(tests.SliceToSeq(tasks)) {
				count++
			}

			So(count, ShouldEqual, 128)
			So(executed.Load(), ShouldEqual, 128)
		})

		Convey("Idle retirement between waves does not lose late tasks", func() {
			var executed atomic.Int64

			waves := func(yield func(unsafe.Pointer) bool) {
				first := []int{1, 2, 3, 4}

				for index := range first {
					if !yield(unsafe.Pointer(&first[index])) {
						return
					}
				}

				// Long enough for every wave-one worker to retire.
				time.Sleep(100 * time.Millisecond)

				second := []int{5, 6, 7, 8}

				for index := range second {
					if !yield(unsafe.Pointer(&second[index])) {
						return
					}
				}
			}

			op := NewPool(func(task *int) {
				executed.Add(1)
				time.Sleep(10 * time.Millisecond)
			}, 20*time.Millisecond)

			count := 0

			for range op.Next(waves) {
				count++
			}

			So(count, ShouldEqual, 8)
			So(executed.Load(), ShouldEqual, 8)
			So(op.Error(), ShouldBeNil)
		})

		Convey("Early consumer termination stops delivery without panic", func() {
			tasks := make([]int, 32)
			op := NewPool(func(task *int) { *task = 7 }, 40*time.Millisecond)
			count := 0

			for range op.Next(tests.SliceToSeq(tasks)) {
				count++
				break
			}

			So(count, ShouldEqual, 1)

			time.Sleep(50 * time.Millisecond)
			So(op.Error(), ShouldBeNil)
		})

		Convey("A second stream after the first is rejected", func() {
			op := NewPool(func(task *int) {}, 40*time.Millisecond)

			for range op.Next(tests.SliceToSeq([]int{1, 2, 3})) {
			}

			count := 0

			for range op.Next(tests.SliceToSeq([]int{9})) {
				count++
			}

			So(count, ShouldEqual, 0)
			So(op.Error(), ShouldNotBeNil)
		})
	})
}

func TestPoolError(t *testing.T) {
	Convey("Given pool construction", t, func() {
		Convey("A non-positive idle lifetime is rejected", func() {
			op := NewPool(func(task *int) {}, 0)

			So(op.Error(), ShouldNotBeNil)

			count := 0

			for range op.Next(tests.SliceToSeq([]int{1})) {
				count++
			}

			So(count, ShouldEqual, 0)
		})

		Convey("A valid pool records no error across a full stream", func() {
			op := NewPool(func(task *int) {}, 40*time.Millisecond)

			for range op.Next(tests.SliceToSeq([]int{1})) {
			}

			So(op.Error(), ShouldBeNil)
		})
	})
}
