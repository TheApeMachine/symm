package compute

import (
	"sync"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWithMetalInit(t *testing.T) {
	Convey("Given concurrent Metal init callers", t, func() {
		var active atomic.Int32
		var peak atomic.Int32
		var calls atomic.Int32
		var initFailed atomic.Bool

		const workers = 16
		waitGroup := sync.WaitGroup{}
		waitGroup.Add(workers)

		for worker := 0; worker < workers; worker++ {
			go func() {
				defer waitGroup.Done()

				initErr := WithMetalInit(func() error {
					calls.Add(1)
					current := active.Add(1)

					for {
						observed := peak.Load()

						if current <= observed {
							break
						}

						if peak.CompareAndSwap(observed, current) {
							break
						}
					}

					active.Add(-1)

					return nil
				})

				if initErr != nil {
					initFailed.Store(true)
				}
			}()
		}

		waitGroup.Wait()

		Convey("It should run init one caller at a time", func() {
			So(initFailed.Load(), ShouldBeFalse)
			So(calls.Load(), ShouldEqual, workers)
			So(peak.Load(), ShouldEqual, 1)
		})
	})
}
