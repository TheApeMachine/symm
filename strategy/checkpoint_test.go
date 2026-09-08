package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
The checkpoint is the one thing a restart must not lose. This pins the
round-trip shape: a model that has learned something serializes it, and a fresh
model restores exactly that learned evidence rather than an empty shell.
*/
func TestCheckpointRoundTrip(t *testing.T) {
	Convey("A learned model survives a checkpoint round-trip", t, func() {
		model := NewEconomicModel()
		key := [2]string{"TEST/USD", "flat"}
		context := []uint64{101, 102}
		enter := LearningAction{Kind: types.ActionEnter}

		So(model.Observe(
			key, context, enter, 0.02, 2.0, 1.0, [2]string{"", "flat"},
		), ShouldBeNil)

		checkpoint := model.Checkpoint()

		So(checkpoint.Version, ShouldEqual, CheckpointVersion)
		So(checkpoint.Priors, ShouldBeGreaterThan, 0)

		restored := NewEconomicModel()

		So(restored.Restore(checkpoint), ShouldBeNil)

		recheckpoint := restored.Checkpoint()

		So(recheckpoint.Priors, ShouldEqual, checkpoint.Priors)
		So(recheckpoint.Nodes, ShouldEqual, checkpoint.Nodes)

		Convey("A checkpoint from another version is refused, not adapted", func() {
			checkpoint.Version = "a-different-build"

			So(NewEconomicModel().Restore(checkpoint), ShouldNotBeNil)
		})
	})
}

func TestEconomicModelRestore(t *testing.T) {
	Convey("Restoration retains signed economics and rejects malformed ownership", t, func() {
		model := NewEconomicModel()
		key := [2]string{"TEST/USD", "holding"}
		action := LearningAction{Kind: types.ActionExit, Reduce: true}
		context := []uint64{2, FrameDelimiter, 1}
		So(model.Observe(key, context, action, -0.02, 3, 1), ShouldBeNil)
		So(model.Observe(key, context, action, 0.01, 7, 1), ShouldBeNil)
		expected := model.Recall(key, context, action)
		checkpoint := model.Checkpoint()
		restored := NewEconomicModel()
		So(restored.Restore(checkpoint), ShouldBeNil)
		So(restored.Recall(key, context, action), ShouldResemble, expected)

		Convey("A forward parent reference cannot partially replace existing knowledge", func() {
			checkpoint.Scopes[0].Nodes[1].Parent = 1
			So(restored.Restore(checkpoint), ShouldNotBeNil)
			So(restored.Recall(key, context, action), ShouldResemble, expected)
		})
		Convey("Declared counts must match the complete object", func() {
			checkpoint.Nodes++
			So(restored.Restore(checkpoint), ShouldNotBeNil)
		})
	})
}

func BenchmarkEconomicModelCheckpoint(b *testing.B) {
	model := NewEconomicModel()
	// A long ordered temporal context checks that checkpoint work and shape are
	// proportional to trie nodes, without repeating every prefix as a full path.
	context := make([]uint64, 256)
	for index := range context {
		context[index] = uint64(index + 1)
	}
	for _, kind := range []types.Action{types.ActionEnter, types.ActionHold, types.ActionExit, types.ActionScale} {
		if err := model.Observe([2]string{"TEST/USD", "holding"}, context, LearningAction{Kind: kind}, 0.01, 30, 1); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		checkpoint := model.Checkpoint()
		if checkpoint.Nodes != len(context)+1 {
			b.Fatal("checkpoint lost context prefixes")
		}
	}
}
