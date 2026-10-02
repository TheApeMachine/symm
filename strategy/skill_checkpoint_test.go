package strategy

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestSkillCheckpointRoundTrip(t *testing.T) {
	Convey("Given observed skill moments from recordSkill", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.recordSkill(true)
		training.recordSkill(true)
		training.recordSkill(false)

		encoded, err := training.skillCheckpoint()
		So(err, ShouldBeNil)
		So(len(encoded), ShouldBeGreaterThan, 0)

		// Fresh training must not invent skill — empty until apply.
		restored := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		// Two +1 samples → mean 1, lower bound > 0 (same as live grading path).
		training.recordSkill(true)
		training.recordSkill(true)

		encoded, err := training.skillCheckpoint()
		So(err, ShouldBeNil)

		again := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		err := training.applySkillCheckpoint([]byte(`{"version":2,"skill":{"Count":2,"Mean":"NaN","M2":1}}`))
		So(err, ShouldNotBeNil)
		So(training.skill, ShouldResemble, statistic.Moments{})
	})
}

func TestSkillCheckpointRejectsOldSupervisionVersion(t *testing.T) {
	Convey("Given a skill blob from old supervision rules, restore is refused", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		err := training.applySkillCheckpoint([]byte(`{"version":1,"skill":{"Count":2,"Mean":1,"M2":0},"returns":{"Count":1,"Mean":0.01,"M2":0}}`))
		So(err, ShouldNotBeNil)
		So(training.skill.Count, ShouldEqual, 0)
		So(training.returns.Count, ShouldEqual, 0)

		err = training.applySkillCheckpoint([]byte(`{"skill":{"Count":2,"Mean":1,"M2":0}}`))
		So(err, ShouldNotBeNil)
		So(training.skill.Count, ShouldEqual, 0)
	})
}

func TestSkillCheckpointLegacyMissingIsZero(t *testing.T) {
	Convey("Missing skill blob leaves moments empty (no invention)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		So(training.skill.Count, ShouldEqual, 0)
		_, _, detail := training.stage()
		So(detail, ShouldEqual, "no graded skill outcomes yet")
	})
}

func TestRecordSkillMarksModelDirty(t *testing.T) {
	Convey("recordSkill dirties the model so save persists skill", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.mu.Lock()
		So(training.modelRevision, ShouldEqual, 0)
		training.mu.Unlock()

		training.recordSkill(true)

		training.mu.Lock()
		dirty := training.modelRevision > 0
		training.mu.Unlock()
		So(dirty, ShouldBeTrue)
	})
}

func TestSkillCheckpointRestoresHistGrades(t *testing.T) {
	Convey("hist enter/exit/wait counters and return samples survive skill checkpoint", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.recordSkill(true)
		training.recordSkill(false)
		training.noteEnterGrade(true, false)  // missed
		training.noteEnterGrade(false, false) // correct wait
		training.noteEnterGrade(true, true)   // correct enter
		training.noteExitGrade(true)
		training.mu.Lock()
		training.recordPolicyReturnLocked(0.0, tables.ExcursionRecord{ExitTick: 5, AnchorTick: 2})
		training.recordPolicyReturnLocked(-0.00013, tables.ExcursionRecord{ExitTick: 6, AnchorTick: 3})
		training.mu.Unlock()

		encoded, err := training.skillCheckpoint()
		So(err, ShouldBeNil)

		again := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		So(again.applySkillCheckpoint(encoded), ShouldBeNil)
		So(again.histMissedEnter, ShouldEqual, 1)
		So(again.histCorrectWait, ShouldEqual, 1)
		So(again.histCorrectEnter, ShouldEqual, 1)
		So(again.histCorrectExit, ShouldEqual, 1)
		So(again.returns.Count, ShouldEqual, 2)
		So(len(again.returnSamples), ShouldEqual, 2)
		So(again.skill.Count, ShouldEqual, training.skill.Count)
	})
}
