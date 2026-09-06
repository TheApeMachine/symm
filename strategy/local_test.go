package strategy

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"testing"
)

func TestLocalLearningTransition(t *testing.T) {
	Convey("Multi-leg book replay preserves each experiment until its measurement closes", t, func() {
		_, events := runTape(t, 8)
		pending := map[int]hindsight.LearningEvent{}
		resolved := 0
		for _, event := range events {
			if event.Mode != "virtual" && event.Mode != "policy" {
				continue
			}
			switch event.Kind {
			case "issued":
				_, exists := pending[event.Lane]
				So(exists, ShouldBeFalse)
				pending[event.Lane] = event
			case "resolved":
				origin, exists := pending[event.Lane]
				So(exists, ShouldBeTrue)
				So(event.ID, ShouldEqual, origin.ID)
				delete(pending, event.Lane)
				resolved++
			}
		}
		So(resolved, ShouldBeGreaterThan, 0)
	})
}
