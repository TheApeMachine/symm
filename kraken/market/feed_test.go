package market

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFeedSubscribe(t *testing.T) {
	Convey("Given a shared feed with one upstream", t, func() {
		sharedFeed := newFeed[int]()
		upstream := make(chan *int, 8)

		ctxOne, cancelOne := context.WithCancel(context.Background())
		defer cancelOne()
		ctxTwo, cancelTwo := context.WithCancel(context.Background())
		defer cancelTwo()

		subOne := sharedFeed.subscribe(ctxOne, func() <-chan *int { return upstream })
		subTwo := sharedFeed.subscribe(ctxTwo, func() <-chan *int { return upstream })

		Convey("It should fan one upstream value out to every subscriber", func() {
			value := 7
			upstream <- &value

			So(*<-subOne, ShouldEqual, 7)
			So(*<-subTwo, ShouldEqual, 7)
		})

		Convey("It should detach and close a subscriber when its context is canceled", func() {
			cancelOne()

			closed := false

			for attempt := 0; attempt < 50 && !closed; attempt++ {
				select {
				case _, ok := <-subOne:
					closed = !ok
				default:
					time.Sleep(time.Millisecond)
				}
			}

			So(closed, ShouldBeTrue)

			value := 9
			upstream <- &value
			So(*<-subTwo, ShouldEqual, 9)
		})
	})
}
