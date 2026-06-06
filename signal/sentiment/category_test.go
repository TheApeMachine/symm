package sentiment

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestSentimentReading(t *testing.T) {
	Convey("Given broad risk-on breadth", t, func() {
		category, evidence := sentimentReading(0.8, 0.1, 0.6, false)

		Convey("It should classify risk-on surge", func() {
			So(category, ShouldEqual, types.CategoryRiskOnSurge)
			So(evidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a leader divergent move", t, func() {
		category, evidence := sentimentReading(0.3, 0.2, 0.6, true)

		Convey("It should classify divergent move", func() {
			So(category, ShouldEqual, types.CategoryDivergentMove)
			So(evidence, ShouldBeGreaterThan, 0)
		})
	})
}
