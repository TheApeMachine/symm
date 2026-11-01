package stoploss

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLearningTrailRoundTrip(t *testing.T) {
	Convey("Learning trail extremes survive JSON and Rebind", t, func() {
		rails := NewRails()
		learning := NewLearning(rails)
		learning.Arm(0)

		_, _ = learning.Update(0.2)
		_, _ = learning.Update(1.8)
		_, _ = learning.Update(1.1)

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
