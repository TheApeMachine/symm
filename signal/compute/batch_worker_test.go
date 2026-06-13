package compute

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBatchWorkerAppliesTasksOffHotPath(t *testing.T) {
	Convey("Given a batch worker with a short interval", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		worker := NewBatchWorker(ctx, 8, 10*time.Millisecond)
		defer worker.Close()

		var applied atomic.Int64

		So(worker.Submit(func() { applied.Add(1) }), ShouldBeTrue)

		time.Sleep(25 * time.Millisecond)

		Convey("It should apply queued work on the worker goroutine", func() {
			So(applied.Load(), ShouldEqual, 1)
		})
	})
}

func BenchmarkBatchWorkerSubmit(benchmark *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := NewBatchWorker(ctx, 8192, 50*time.Millisecond)
	defer worker.Close()

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		worker.Submit(func() {})
	}
}
