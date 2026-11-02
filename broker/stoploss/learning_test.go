package stoploss

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLearningTrailRoundTrip(t *testing.T) {
	Convey("Learning trail extremes survive JSON and Rebind", t, func() {
		rails := NewRails()
		learning := NewLearning(rails)
		learning.Arm(0.01)

		_, _ = learning.Update(0.2, Book{})
		_, _ = learning.Update(1.8, Book{})
		_, _ = learning.Update(1.1, Book{})

		raw, err := learning.MarshalJSON()
		So(err, ShouldBeNil)

		restored := &Learning{}
		So(restored.UnmarshalJSON(raw), ShouldBeNil)
		restored.Rebind(rails)

		So(restored.stop.Machine().Initialized, ShouldBeTrue)
		So(restored.stop.Machine().PeakPnL, ShouldEqual, 1.8)
		So(restored.armed, ShouldBeTrue)
	})
}

func TestLearningArmIgnoresZeroFriction(t *testing.T) {
	Convey("Arm(0) does not freeze the shadow rails", t, func() {
		learning := NewLearning(NewRails())

		learning.Arm(0)
		So(learning.armed, ShouldBeFalse)

		learning.Arm(0.015)
		So(learning.armed, ShouldBeTrue)
	})
}

func TestLearningBookExhaustion(t *testing.T) {
	Convey("thin touch bids exit a green shadow lot", t, func() {
		learning := NewLearning(NewRails())
		learning.Arm(0.005)

		_, _ = learning.Update(0.2, Book{})
		_, _ = learning.Update(3.0, Book{ExitRoom: 2.0, Backing: 0.8})

		trigger, ok := learning.Update(2.8, Book{ExitRoom: 0.5, Backing: 0.8})
		So(ok, ShouldBeTrue)
		So(trigger, ShouldContainSubstring, "shadow book exhaustion")
	})
}
