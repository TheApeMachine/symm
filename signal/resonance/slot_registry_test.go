package resonance

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSlotRegistryAssign(testingTB *testing.T) {
	Convey("Given a slot registry", testingTB, func() {
		Convey("It should keep stable slots per symbol under concurrent assign", func() {
			registry := newSlotRegistry(4)

			const workers = 32
			waitGroup := sync.WaitGroup{}
			waitGroup.Add(workers)

			slots := make([]int, workers)

			for worker := 0; worker < workers; worker++ {
				worker := worker

				go func() {
					defer waitGroup.Done()

					slot, ok := registry.assign("BTC/USD")

					if ok {
						slots[worker] = slot + 1
					}
				}()
			}

			waitGroup.Wait()

			So(slots[0], ShouldBeGreaterThan, 0)

			for _, slot := range slots {
				So(slot, ShouldEqual, slots[0])
			}
		})

		Convey("It should stop assigning beyond capacity", func() {
			registry := newSlotRegistry(4)

			_, ok := registry.assign("BTC/USD")
			So(ok, ShouldBeTrue)

			_, ok = registry.assign("ETH/USD")
			So(ok, ShouldBeTrue)

			_, ok = registry.assign("SOL/USD")
			So(ok, ShouldBeTrue)

			_, ok = registry.assign("DOGE/USD")
			So(ok, ShouldBeTrue)

			_, ok = registry.assign("ADA/USD")
			So(ok, ShouldBeFalse)
		})

		Convey("It should reclaim slots of departed symbols for reuse", func() {
			registry := newSlotRegistry(2)

			unfiSlot, ok := registry.assign("UNFI/USD")
			So(ok, ShouldBeTrue)

			_, ok = registry.assign("SRM/USD")
			So(ok, ShouldBeTrue)

			// Full: a third pair cannot fit yet.
			_, ok = registry.assign("SLX/USD")
			So(ok, ShouldBeFalse)

			// UNFI leaves the live universe; its slot is freed and returned.
			freed := registry.retain([]string{"SRM/USD"})
			So(freed, ShouldResemble, []int{unfiSlot})

			// The newcomer now reuses the reclaimed slot — no leak, no growth.
			slxSlot, ok := registry.assign("SLX/USD")
			So(ok, ShouldBeTrue)
			So(slxSlot, ShouldEqual, unfiSlot)
		})
	})
}
