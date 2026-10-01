package hindsight_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestStoreTeeBackpressureDoesNotSilentDrop(t *testing.T) {
	Convey("Given a full StoreTee queue, Push waits then fails explicitly on cancel", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		tee := hindsight.NewStoreTee(ctx, "backpressure")
		tee.Transition(runtime.READY)

		// Fill to capacity.
		for i := 0; i < 8192; i++ {
			m := data.NewMeasurement[float64]("websocket", nil)
			m.Label = "BTC/USD"
			m.SeqIdx = int64(i + 1)
			tee.Push(m)
		}
		So(tee.Pending(), ShouldEqual, 8192)
		So(tee.Error(), ShouldBeNil)

		// Next push must not silently drop — cancel while blocked → explicit error.
		done := make(chan struct{})
		go func() {
			m := data.NewMeasurement[float64]("websocket", nil)
			m.Label = "BTC/USD"
			m.SeqIdx = 99999
			tee.Push(m)
			close(done)
		}()

		time.Sleep(20 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Push did not return after cancel")
		}
		So(tee.Error(), ShouldNotBeNil)
		// Queue still full — the blocked push did not enqueue a silent replacement.
		So(tee.Pending(), ShouldEqual, 8192)
	})
}

func TestStoreTeePushNotReadyFailsExplicitly(t *testing.T) {
	Convey("Push on non-READY fails explicitly instead of silent return", t, func() {
		tee := hindsight.NewStoreTee(context.Background(), "not-ready")
		m := data.NewMeasurement[float64]("websocket", nil)
		m.Label = "BTC/USD"
		tee.Push(m)
		So(tee.Error(), ShouldNotBeNil)
		So(tee.Pending(), ShouldEqual, 0)
	})
}
