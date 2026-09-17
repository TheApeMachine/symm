package ui_test

import (
	"context"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/cognition"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

func TestTrainingPublisher(t *testing.T) {
	Convey("TrainingPublisher adapts cognition.Evaluation into wire measurements for UI and store", t, func() {
		ctx := context.Background()
		uiTee := ui.NewUITee(ctx, "test-ui-tee", 64)
		uiTee.Transition(nmruntime.READY)

		storeTee := hindsight.NewStoreTee(ctx, "test-store-tee", 64)
		storeTee.Transition(nmruntime.READY)

		publisher := ui.NewTrainingPublisher(uiTee, storeTee, "BTC/USD")
		types.SetFocus("BTC/USD")
		types.SetRoute("learning")

		evaluation := &cognition.Evaluation{
			Context:     []byte("test-context"),
			Step:        42,
			Support:     100,
			WinnerClass: "enter",
			Confidence:  0.88,
			Contrast:    0.15,
			Surprisal:   0.22,
			Ambiguity:   0.05,
		}

		feed := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(evaluation))
		}

		for range publisher.Next(feed) {
		}

		So(storeTee.Pending(), ShouldEqual, 1)

		time.Sleep(10 * time.Millisecond)
		frame := uiTee.Next()
		So(frame, ShouldNotBeNil)

		payload := *(*[]byte)(frame)
		So(len(payload), ShouldBeGreaterThan, 0)
	})
}
