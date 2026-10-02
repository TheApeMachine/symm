package strategy

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/ui"
)

func TestReporter(t *testing.T) {
	Convey("Reporter telemetry publisher", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		arena := data.NewArenaOwner(4096)
		uiTee := ui.NewUITee(ctx, "ui-test", 1, func(_ *data.Measurement[float64]) bool { return true })
		uiTee.Transition(runtime.READY)

		reporter := NewReporter(arena, uiTee)
		skill := NewSkill()
		skill.Record(0.015)
		skill.Record(0.025)

		Convey("Publishes historical training frame with expected metrics", func() {
			snapshot := ReportSnapshot{
				Source:       "training:historical",
				Symbol:       "XXBTZUSD",
				SeqIdx:       105,
				At:           time.Now(),
				Stage:        StageHistoricalValidation,
				Blocker:      "gathering statistical evidence",
				Action:       1,
				Confidence:   0.85,
				Contrast:     1.5,
				Tokens:       []byte{1, 2, 5},
				MarkA:        90,
				MarkB:        100,
				MarkC:        120,
				Price:        65000.50,
				ExcursionMag: 0.05,
				Direction:    "up",
				Clears:       true,
				Event:        "completed",
				Trading:      false,
			}

			reporter.Publish(snapshot, skill)

			frame := uiTee.Next()
			So(frame, ShouldNotBeNil)

			payload := *(*[]byte)(frame)
			So(len(payload), ShouldBeGreaterThan, 0)
		})
	})
}
