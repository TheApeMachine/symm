package strategy

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a planner thesis gate", t, func() {
		planner := &Planner{subscribers: &sync.Map{}}
		thesis := types.NewThesis()
		thesis.Status = types.READY

		Convey("It should wait until manifold, resonance, causal, and graph are ready", func() {
			returned := planner.Update(thesis)

			So(returned, ShouldEqual, thesis)
			So(len(thesis.Decisions), ShouldEqual, 0)
		})
	})
}