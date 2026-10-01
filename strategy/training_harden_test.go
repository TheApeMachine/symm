package strategy

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestFramesRangeExitIsBToC(t *testing.T) {
	Convey("framesRange keeps [B, C) for exit learning", t, func() {
		frames := []*data.Measurement[float64]{
			{SeqIdx: 1}, // before B
			{SeqIdx: 4}, // B
			{SeqIdx: 5},
			{SeqIdx: 7}, // last before C
			{SeqIdx: 8}, // C excluded
		}
		got := framesRange(frames, 4, 8)
		So(len(got), ShouldEqual, 3)
		So(got[0].SeqIdx, ShouldEqual, 4)
		So(got[2].SeqIdx, ShouldEqual, 7)

		beforeB := framesBefore(frames, 4)
		So(len(beforeB), ShouldEqual, 1)
		So(beforeB[0].SeqIdx, ShouldEqual, 1)
	})
}

func TestSuperviseExitUsesBToCNotFullPath(t *testing.T) {
	Convey("Cold engine: one eligible B→C episode teaches EXIT without seed prediction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)

		frames := []*data.Measurement[float64]{
			regionFrame("BTC/USD", 1, 2),
			regionFrame("BTC/USD", 2, 3),
			regionFrame("BTC/USD", 4, 10),
			regionFrame("BTC/USD", 5, 11),
			regionFrame("BTC/USD", 7, 12),
		}
		for _, f := range frames {
			training.grid.Update(f)
		}
		training.grid.Settle()

		bToC := framesRange(frames, 4, 8)
		aToC := framesBefore(frames, 8)
		So(len(bToC), ShouldBeLessThan, len(aToC))
		So(len(bToC), ShouldEqual, 3)

		exitCtx := training.signatureOf(bToC)
		So(len(exitCtx), ShouldBeGreaterThan, 0)

		// Cold engine: zero EXIT associations — freeze must not already predict EXIT.
		before := training.engine.Census()
		So(before["exit"], ShouldEqual, int32(0))
		So(training.frozenAction(exitCtx), ShouldNotEqual, cognition.ActionExit)

		episode := heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:exit:1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				ClearsFriction:     true,
				PrecursorStartTick: 1,
				AnchorTick:         4,
				ExitTick:           8,
				EntryPrice:         100,
				ExitPrice:          101,
			},
			frames: frames,
		}
		training.supervise(episode, true)

		// Ground-truth teach from resolved C installs EXIT even when freeze missed.
		after := training.engine.Census()
		So(after["exit"], ShouldBeGreaterThan, int32(0))
		So(training.frozenAction(exitCtx), ShouldEqual, cognition.ActionExit)
		// Cold freeze missed EXIT → grade as miss; skill still recorded.
		So(training.histMissedExit, ShouldEqual, 1)
		So(training.histCorrectExit, ShouldEqual, 0)
		So(training.skill.Count, ShouldBeGreaterThan, 0)
	})
}

func TestWriteEpisodeStampsMarksBeforeExitTick(t *testing.T) {
	Convey("Historical developing frames carry A/B/C marks", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		clone := regionFrame("BTC/USD", 5, 2)
		record := &tables.ExcursionRecord{
			PrecursorStartTick: 1,
			AnchorTick:         4,
			ExitTick:           8,
			Direction:          "up",
			ClearsFriction:     true,
			ProfitFraction:     0.01,
		}

		training.writeEpisode(clone, record)
		So(clone.Metadata["excursion_event"], ShouldEqual, "developing")
		So(clone.GetMetric("mark_a").Raw, ShouldEqual, 1)
		So(clone.GetMetric("mark_b").Raw, ShouldEqual, 4)
		So(clone.GetMetric("mark_c").Raw, ShouldEqual, 8)

		done := regionFrame("BTC/USD", 8, 2)
		training.writeEpisode(done, record)
		So(done.Metadata["excursion_event"], ShouldEqual, "completed")
	})
}

func TestStageBlocksPaperDuringHistoricalReplay(t *testing.T) {
	Convey("replaying keeps HISTORICAL VALIDATION even with positive skill", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		training.recordSkill(true)
		training.recordSkill(true)

		training.mu.Lock()
		training.checkpointed = true
		training.replaying = true
		training.mu.Unlock()

		So(training.paperOpen(), ShouldBeFalse)
		_, stage, detail := training.stage()
		So(stage, ShouldEqual, "HISTORICAL VALIDATION")
		So(detail, ShouldEqual, "historical replay in progress")

		training.mu.Lock()
		training.replaying = false
		training.mu.Unlock()
		So(training.paperOpen(), ShouldBeTrue)
	})
}
