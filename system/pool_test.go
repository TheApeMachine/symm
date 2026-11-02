package system

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/panjf2000/ants/v2"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPoolSubmit(t *testing.T) {
	Convey("Given a closed pool", t, func() {
		pool := NewPool()
		pool.handle.Release()

		ran := false
		err := pool.Submit(func() { ran = true })

		Convey("Submit returns the rejection and does not run the task", func() {
			So(err, ShouldEqual, ants.ErrPoolClosed)
			So(ran, ShouldBeFalse)
		})
	})

	Convey("Given a rejected submit that owns a wait group", t, func() {
		pool := NewPool()
		pool.handle.Release()

		var wg sync.WaitGroup
		var ran atomic.Int32
		wg.Add(1)

		task := func() {
			defer wg.Done()
			ran.Add(1)
		}

		if pool.Submit(task) != nil {
			task()
		}

		wg.Wait()

		Convey("The caller runs the task so Wait can return", func() {
			So(ran.Load(), ShouldEqual, 1)
		})
	})

	Convey("Given a live pool", t, func() {
		pool := NewPool()
		defer pool.handle.Release()

		var wg sync.WaitGroup
		wg.Add(1)
		err := pool.Submit(func() { wg.Done() })
		wg.Wait()

		Convey("Submit accepts the task", func() {
			So(err, ShouldBeNil)
		})
	})
}
