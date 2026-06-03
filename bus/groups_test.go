package bus

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestGroup(t *testing.T) {
	Convey("Given two Group calls for the same id on one pool", t, func() {
		pool := qpool.NewQ(context.Background(), 1, 4, nil)
		defer pool.Close()

		first := Group(pool, "ui", 100*time.Millisecond)
		second := Group(pool, "ui", 10*time.Millisecond)

		Convey("It should return the same broadcast group", func() {
			So(first, ShouldNotBeNil)
			So(second, ShouldEqual, first)
		})

		Convey("It should deliver publishes to subscribers on that group", func() {
			subscriber := first.Subscribe("bus:test", 4)
			first.Send(&qpool.QValue[any]{Value: map[string]any{"type": "fluid"}})

			select {
			case message := <-subscriber.Incoming:
				So(message, ShouldNotBeNil)
				So(message.Value, ShouldResemble, map[string]any{"type": "fluid"})
			case <-time.After(2 * time.Second):
				So("timeout waiting for bus fanout", ShouldBeEmpty)
			}
		})
	})
}
