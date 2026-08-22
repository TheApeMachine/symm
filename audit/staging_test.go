package audit

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestStager(t *testing.T) {
	Convey("Given a new decision stager", t, func() {
		stager := NewStager(nil)
		So(stager, ShouldNotBeNil)

		decision := &types.Decision{
			ID:     "decision-1",
			Symbol: "BTC/USD",
			Action: types.ActionEnter,
		}

		Convey("Staging a decision should hold it in memory", func() {
			stager.Stage(decision, 50*time.Millisecond)
			So(stager.Matured(), ShouldBeEmpty)

			time.Sleep(60 * time.Millisecond)
			matured := stager.Matured()
			So(matured, ShouldHaveLength, 1)
			So(matured[0].ID, ShouldEqual, "decision-1")
		})

		Convey("Pruning a decision should remove it from memory", func() {
			stager.Stage(decision, 10*time.Millisecond)
			stager.Prune("decision-1")
			time.Sleep(20 * time.Millisecond)
			So(stager.Matured(), ShouldBeEmpty)
		})
	})
}
