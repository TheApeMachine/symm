package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLearningStopDoesNotShareLiveSettle(t *testing.T) {
	Convey("learning fires as a shadow episode without needing structural", t, func() {
		rails := NewAdaptiveRails()
		learning := NewLearningStop(rails)
		learning.Arm(0)

		_, armed := learning.Update(0.0)
		So(armed, ShouldBeFalse)

		learning.Update(3.0)
		trigger, fired := learning.Update(1.0)
		So(fired, ShouldBeTrue)
		So(trigger, ShouldContainSubstring, "trailing take-profit")
		So(learning.done, ShouldBeTrue)

		learning.Settle(0.4)
		So(rails.Episodes(), ShouldEqual, 1)
	})
}

func TestLearningStopSettleOnce(t *testing.T) {
	Convey("Settle is idempotent so open+post paths cannot double-learn", t, func() {
		rails := NewAdaptiveRails()
		learning := NewLearningStop(rails)
		learning.Arm(0)
		learning.Update(0.0)
		learning.Update(-3.0)

		learning.Settle(0.0)
		learning.Settle(0.0)
		So(rails.Episodes(), ShouldEqual, 1)
	})
}
