package runtime

import (
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

// awaitWorkers polls GetSpawnedWorkers until it satisfies the predicate or the
// timeout elapses, returning the last observed value.
func awaitWorkers(elasticPool *Pool[int], predicate func(int) bool, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)

	for {
		observed := elasticPool.GetSpawnedWorkers()
		if predicate(observed) {
			return observed
		}

		if time.Now().After(deadline) {
			return observed
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func NewPoolTest(t *testing.T) {
	Convey("Given a handler function", t, func() {
		handlerFunc := func(task int) {}

		Convey("A fresh pool defaults to a one second idle lifetime", func() {
			loadPool := NewPool(handlerFunc)

			So(loadPool.handlerFunc, ShouldNotBeNil)
			So(loadPool.idleWorkerLifetime, ShouldEqual, time.Second)
		})
	})
}

func SetIdleWorkerLifetimeTest(t *testing.T) {
	Convey("Given an elastic pool", t, func() {
		loadPool := NewPool(func(task int) {})
		loadPool.Start()

		Convey("A positive lifetime is honored", func() {
			loadPool.SetIdleWorkerLifetime(75 * time.Millisecond)

			So(loadPool.idleWorkerLifetime, ShouldEqual, 75*time.Millisecond)
		})

		Convey("A non-positive lifetime falls back to one second", func() {
			loadPool.SetIdleWorkerLifetime(-time.Second)

			So(loadPool.idleWorkerLifetime, ShouldEqual, time.Second)
		})

		Reset(func() {
			loadPool.StopAndWait()
		})
	})
}

func GetSpawnedWorkersTest(t *testing.T) {
	Convey("Given a freshly started pool", t, func() {
		loadPool := NewPool(func(task int) {
			time.Sleep(20 * time.Millisecond)
		})
		loadPool.SetIdleWorkerLifetime(40 * time.Millisecond)
		loadPool.Start()

		Convey("A burst of blocked tasks scales workers up", func() {
			for task := 0; task < 12; task++ {
				err := loadPool.AddTask(task)
				So(err, ShouldBeNil)
			}

			peak := awaitWorkers(loadPool, func(current int) bool { return current >= 2 }, 2*time.Second)
			So(peak, ShouldBeGreaterThanOrEqualTo, 2)
		})

		Convey("After the burst drains, idle retirement scales back to zero", func() {
			for task := 0; task < 12; task++ {
				So(loadPool.AddTask(task), ShouldBeNil)
			}

			awaitWorkers(loadPool, func(current int) bool { return current >= 2 }, 2*time.Second)
			floor := awaitWorkers(loadPool, func(current int) bool { return current == 0 }, 3*time.Second)
			So(floor, ShouldEqual, 0)
		})

		Reset(func() {
			loadPool.StopAndWait()
		})
	})
}

func StartTest(t *testing.T) {
	Convey("Given an unstarted pool", t, func() {
		loadPool := NewPool[int](func(task int) {})

		Convey("Submitting before Start is rejected", func() {
			err := loadPool.AddTask(1)
			So(err, ShouldNotBeNil)
		})

		Convey("Start is idempotent", func() {
			loadPool.Start()
			loadPool.Start()

			So(loadPool.started.Load(), ShouldBeTrue)
		})

		Reset(func() {
			loadPool.StopAndWait()
		})
	})
}

func AddTaskTest(t *testing.T) {
	Convey("Given a started elastic pool", t, func() {
		processed := make(chan int, 256)
		loadPool := NewPool[int](func(task int) {
			processed <- task
		})
		loadPool.SetIdleWorkerLifetime(40 * time.Millisecond)
		loadPool.Start()

		Convey("Every task is delivered exactly once", func() {
			for task := 0; task < 64; task++ {
				So(loadPool.AddTask(task), ShouldBeNil)
			}

			seen := make([]bool, 64)
			for received := 0; received < 64; received++ {
				var task int
				select {
				case task = <-processed:
				case <-time.After(5 * time.Second):
					So(false, ShouldBeTrue, "pool consumed too few tasks within the timeout")
				}

				if task < 0 || task >= 64 {
					So(false, ShouldBeTrue, "received an unexpected task value")
				}

				if seen[task] {
					So(false, ShouldBeTrue, "task was delivered more than once")
				}

				seen[task] = true
			}

			for received := 0; received < 64; received++ {
				So(seen[received], ShouldBeTrue)
			}
		})

		Convey("Tasks are rejected after Stop", func() {
			loadPool.Stop()
			err := loadPool.AddTask(1)

			So(err, ShouldNotBeNil)
		})

		Reset(func() {
			loadPool.StopAndWait()
		})
	})
}

func AddTaskWithBlockingTest(t *testing.T) {
	Convey("Given a started elastic pool", t, func() {
		var executed int64
		loadPool := NewPool[int](func(task int) {
			atomic.AddInt64(&executed, 1)
		})
		loadPool.Start()

		Convey("A un-bounded queue accepts immediately", func() {
			err := loadPool.AddTaskWithBlocking(7)

			So(err, ShouldBeNil)
		})

		Reset(func() {
			loadPool.StopAndWait()
		})
	})
}

func StopTest(t *testing.T) {
	Convey("Given a started pool with no traffic", t, func() {
		loadPool := NewPool[int](func(task int) {})
		loadPool.Start()

		Convey("StopAndWait returns promptly even with zero live workers", func() {
			deadline := time.Now().Add(2 * time.Second)
			loadPool.StopAndWait()

			So(time.Now().Before(deadline), ShouldBeTrue)
		})
	})
}

func StopAndWaitTest(t *testing.T) {
	Convey("Given a started pool", t, func() {
		var executed atomic.Int64
		loadPool := NewPool[int](func(task int) {
			executed.Add(1)
		})
		loadPool.SetIdleWorkerLifetime(40 * time.Millisecond)
		loadPool.Start()

		Convey("StopAndWait drains every queued task before returning", func() {
			for task := 0; task < 128; task++ {
				err := loadPool.AddTask(task)
				So(err, ShouldBeNil)
			}

			loadPool.StopAndWait()

			So(executed.Load(), ShouldEqual, 128)
		})

		Reset(func() {
			loadPool.StopAndWait()
		})
	})
}

func StopWithTimeoutTest(t *testing.T) {
	Convey("Given a started pool", t, func() {
		loadPool := NewPool[int](func(task int) {})
		loadPool.Start()

		Convey("StopWithTimeout reports a clean drain", func() {
			completed := loadPool.StopWithTimeout(2 * time.Second)

			So(completed, ShouldBeTrue)
		})
	})
}
