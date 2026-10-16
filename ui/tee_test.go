package ui_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
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

		Convey("It admits allowed publications and encodes them", func() {
			value := data.NewValue("hawkes", "signal", "BTC/USD")
			

			tee.Push(value)

			time.Sleep(20 * time.Millisecond)

			frame := tee.Next()
			So(frame, ShouldNotBeNil)
			payload := *(*[]byte)(frame)
			So(len(payload), ShouldBeGreaterThan, 0)
		})
	})
}
