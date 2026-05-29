package sentiment

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestSentimentCategory(t *testing.T) {
	Convey("Given breadth and leadership context", t, func() {
		Convey("It should classify risk-on surge", func() {
			So(sentimentCategory(0.6, 0.03, 0.04), ShouldEqual, engine.CatRiskOnSurge)
		})

		Convey("It should classify divergent move", func() {
			So(sentimentCategory(0.3, 0.04, 0.04), ShouldEqual, engine.CatDivergentMove)
		})

		Convey("It should classify systemic slump", func() {
			So(sentimentCategory(0.3, 0.01, 0.04), ShouldEqual, engine.CatSystemicSlump)
		})
	})
}
