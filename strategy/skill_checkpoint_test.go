package strategy

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestSkillCheckpointRoundTrip(t *testing.T) {
	Convey("Given observed skill moments from recordSkill", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.recordSkill(true)
		training.recordSkill(true)
		training.recordSkill(false)

		encoded, err := training.skillCheckpoint()
		So(err, ShouldBeNil)
		So(len(encoded), ShouldBeGreaterThan, 0)

		// Fresh training must not invent skill — empty until apply.
		restored := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		So(restored.skill.Count, ShouldEqual, 0)

		So(restored.applySkillCheckpoint(encoded), ShouldBeNil)
		So(restored.skill.Count, ShouldEqual, training.skill.Count)
		So(restored.skill.Mean, ShouldEqual, training.skill.Mean)
		So(restored.skill.M2, ShouldEqual, training.skill.M2)
	})
}

func TestSkillCheckpointRestoresPaperOpenGate(t *testing.T) {
	Convey("Given a restored positive skill lower bound without re-supervise", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		// Two +1 samples → mean 1, lower bound > 0 (same as live grading path).
		training.recordSkill(true)
		training.recordSkill(true)

		encoded, err := training.skillCheckpoint()
		So(err, ShouldBeNil)

		again := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		again.grid.Update(frame)
		again.grid.Settle()
		again.mu.Lock()
		again.checkpointed = true
		again.mu.Unlock()

		So(again.paperOpen(), ShouldBeFalse)
		So(again.applySkillCheckpoint(encoded), ShouldBeNil)
		So(again.paperOpen(), ShouldBeTrue)

		_, stage, detail := again.stage()
		So(stage, ShouldEqual, "FORWARD PAPER LEARNING")
		So(detail, ShouldEqual, "")
	})
}

func TestSkillCheckpointRejectsNaN(t *testing.T) {
	Convey("Given corrupt moments, apply fails without writing skill", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		err := training.applySkillCheckpoint([]byte(`{"skill":{"Count":2,"Mean":"NaN","M2":1}}`))
		So(err, ShouldNotBeNil)
		So(training.skill, ShouldResemble, statistic.Moments{})
	})
}

func TestSkillCheckpointLegacyMissingIsZero(t *testing.T) {
	Convey("Missing skill blob leaves moments empty (no invention)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		So(training.skill.Count, ShouldEqual, 0)
		_, _, detail := training.stage()
		So(detail, ShouldEqual, "skill lower bound not positive")
	})
}

func TestRecordSkillMarksModelDirty(t *testing.T) {
	Convey("recordSkill dirties the model so save persists skill", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.mu.Lock()
		So(training.modelDirty, ShouldBeFalse)
		training.mu.Unlock()

		training.recordSkill(true)

		training.mu.Lock()
		dirty := training.modelDirty
		training.mu.Unlock()
		So(dirty, ShouldBeTrue)
	})
}
