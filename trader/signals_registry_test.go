package trader

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInitSignals(testingTB *testing.T) {
	Convey("Given a crypto trader", testingTB, func() {
		pool := productionPool(testingTB)
		crypto := NewCrypto(context.Background(), pool)

		defer pool.Close()
		defer crypto.Close()

		Convey("It should expose stable signal pointers under concurrent reads", func() {
			const workers = 32
			waitGroup := sync.WaitGroup{}
			waitGroup.Add(workers)

			causalPointer := crypto.causalSignal
			var mismatch atomic.Bool

			for worker := 0; worker < workers; worker++ {
				go func() {
					defer waitGroup.Done()

					if crypto.causalSignal != causalPointer {
						mismatch.Store(true)
					}
				}()
			}

			waitGroup.Wait()

			So(mismatch.Load(), ShouldBeFalse)
			So(crypto.causalSignal, ShouldNotBeNil)
			So(crypto.resonanceSignal, ShouldNotBeNil)
			So(crypto.fluidSignal, ShouldNotBeNil)
		})
	})
}
