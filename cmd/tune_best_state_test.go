package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTuneBestStateUpdateIfBetter(t *testing.T) {
	Convey("Given an empty tune best state", t, func() {
		state := newTuneBestState()
		document := perspectives.Document{Version: 1}
		candidate := tuneCandidate{perspectives: &document}

		Convey("It should accept the first eligible candidate", func() {
			accepted, _ := state.UpdateIfBetter(candidate, trialScores{
				selection:  4,
				trainScore: 3,
			})
			snapshot := state.Snapshot()

			So(accepted, ShouldBeTrue)
			So(snapshot.hasBest, ShouldBeTrue)
			So(snapshot.selection, ShouldEqual, 4)
			So(snapshot.trainScore, ShouldEqual, 3)
		})

		Convey("It should reject a worse holdout candidate", func() {
			state.UpdateIfBetter(candidate, trialScores{selection: 5, trainScore: 2})
			accepted, _ := state.UpdateIfBetter(candidate, trialScores{selection: 1, trainScore: 9})

			So(accepted, ShouldBeFalse)
			So(state.Snapshot().selection, ShouldEqual, 5)
		})
	})
}
