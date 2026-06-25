package compute

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSerialPoolCall(t *testing.T) {
	Convey("Given a serial pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		serial := NewSerialPool(ctx, 8, 10*time.Millisecond)
		defer serial.Close()

		Convey("It should run blocking work on the worker goroutine", func() {
			var workerThread atomic.Bool

			value := Run(serial, func() int {
				workerThread.Store(true)
				return 7
			})

			So(workerThread.Load(), ShouldBeTrue)
			So(value, ShouldEqual, 7)
		})
	})
}

func TestSerialPoolEnqueueOffHotPath(t *testing.T) {
	Convey("Given a serial pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		serial := NewSerialPool(ctx, 8, 10*time.Millisecond)
		defer serial.Close()

		done := make(chan struct{})
		var applied atomic.Bool

		So(serial.Enqueue(func() {
			applied.Store(true)
			close(done)
		}), ShouldBeTrue)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for serial pool task")
		}

		Convey("It should apply the task on the worker goroutine", func() {
			So(applied.Load(), ShouldBeTrue)
		})
	})
}

func TestSerialPoolDroppedTasks(t *testing.T) {
	Convey("Given a saturated serial pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		serial := NewSerialPool(ctx, 1, time.Hour)
		defer serial.Close()

		blocker := make(chan struct{})
		So(serial.Enqueue(func() { <-blocker }), ShouldBeTrue)
		So(serial.Enqueue(func() {}), ShouldBeTrue)

		Convey("It should drop the oldest task and report the drop", func() {
			So(serial.Enqueue(func() {}), ShouldBeTrue)
			So(serial.DroppedTasks(), ShouldEqual, 1)
		})

		close(blocker)
	})
}

func BenchmarkSerialPoolEnqueue(benchmark *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serial := NewSerialPool(ctx, 8192, 50*time.Millisecond)
	defer serial.Close()

	benchmark.ResetTimer()

	for benchmark.Loop() {
		serial.Enqueue(func() {})
	}
}
