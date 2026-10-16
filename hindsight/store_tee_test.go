package hindsight

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestStoreTee_Push(t *testing.T) {
	Convey("Given a StoreTee", t, func() {
		tee := NewStoreTee(context.Background(), "test")
		row := data.NewValue("detector", "excursion", "BTC/USD")

		Convey("A push before READY is dropped and counted", func() {
			So(tee.Status(), ShouldNotEqual, runtime.READY)
			tee.Push(row)
			So(tee.Pending(), ShouldEqual, 0)
		})

		Convey("A push while READY is queued", func() {
			tee.Transition(runtime.READY)
			tee.Push(row)
			So(tee.Pending(), ShouldEqual, 1)
		})
	})

	Convey("Given a running StoreTee, pushed items are drained", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tee := NewStoreTee(ctx, "test_drain")
		tee.Transition(runtime.READY)
		tee.Run()

		for i := 0; i < 10; i++ {
			tee.Push(data.NewValue("test", "metric", "BTC/USD"))
		}

		So(tee.Pending(), ShouldBeGreaterThan, 0)

		// Wait for ticker to flip and workers to drain
		drained := false
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if tee.Pending() == 0 {
				drained = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		So(drained, ShouldBeTrue)
	})
}
