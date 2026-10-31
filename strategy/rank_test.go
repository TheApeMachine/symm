package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmstore "github.com/theapemachine/nomagique/store"
)

func TestRankTakeSorted(t *testing.T) {
	Convey("opportunities commit highest score first", t, func() {
		rank := NewRank()
		rank.Offer(Opportunity{Symbol: "NEAR/USD", Score: 1.0})
		rank.Offer(Opportunity{Symbol: "STRK/USD", Score: 9.0})
		rank.Offer(Opportunity{Symbol: "ONDO/USD", Score: 2.0})

		sorted := rank.TakeSorted()
		So(sorted, ShouldHaveLength, 3)
		So(sorted[0].Symbol, ShouldEqual, "STRK/USD")
		So(sorted[1].Symbol, ShouldEqual, "ONDO/USD")
		So(sorted[2].Symbol, ShouldEqual, "NEAR/USD")
		So(rank.TakeSorted(), ShouldBeNil)
	})
}

func TestScoreFormula(t *testing.T) {
	Convey("score is accel × brightness / spread", t, func() {
		So(Score(2.0, 0.5, 1.0), ShouldEqual, 1.0)
		So(Score(-1.0, 0.5, 1.0), ShouldEqual, 0)
		So(Score(2.0, 0.5, 0), ShouldEqual, 0)
		So(Score(0, 0.5, 1.0), ShouldEqual, 0)
	})

	Convey("peakBrightness takes the lit max", t, func() {
		So(peakBrightness([]*nmstore.Region{
			{Brightness: 0.2},
			{Brightness: 0.7},
			nil,
		}), ShouldEqual, 0.7)
	})
}
