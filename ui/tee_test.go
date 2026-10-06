package ui_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

func TestUITee(t *testing.T) {
	Convey("Given a ready UITee", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		types.SetRoute("dashboard")
		types.SetFocus("BTC/USD")

		tee := ui.NewUITee(ctx, "testTee")
		tee.Transition(nmruntime.READY)

		arena := data.NewArenaOwner("test", 1024)

		Convey("It admits allowed publications and encodes them", func() {
			measurement := arena.NewMeasurement(1, "BTC/USD", "hawkes", 1, 1, nil)
			measurement.At = time.Now()
			measurement.From = measurement.At
			measurement.Write(data.NewMetric("event_count", 1, data.UnitCount, data.TimescaleInstantaneous))

			pub := data.NewPublication(measurement, nil)
			tee.Push(pub)

			time.Sleep(20 * time.Millisecond)

			frame := tee.Next()
			So(frame, ShouldNotBeNil)
			payload := *(*[]byte)(frame)
			So(len(payload), ShouldBeGreaterThan, 0)
		})
	})
}
