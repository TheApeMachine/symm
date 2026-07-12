package strategy

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecisionValidate(t *testing.T) {
	Convey("Given a provenance-complete decision with hold and buy alternatives", t, func() {
		decision := testDecision(1, ActionBuy, 0.05)

		Convey("When its strategy artifact is validated", func() {
			Convey("Then broker can consume the selected action without inference", func() {
				So(decision.Validate(), ShouldBeNil)
			})
		})

		Convey("When the selected utility is absent from the alternatives", func() {
			decision.Utility = 0.06

			Convey("Then the inconsistent selection is rejected", func() {
				So(decision.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When the do-nothing alternative is omitted", func() {
			decision.Alternatives = decision.Alternatives[:1]

			Convey("Then utility comparison is incomplete", func() {
				So(decision.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When an alternative utility is non-finite", func() {
			decision.Alternatives[0].Utility = math.Inf(1)

			Convey("Then the decision cannot enter the journal", func() {
				So(decision.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When forecast provenance has no source", func() {
			decision.Forecast.Source = ""

			Convey("Then the evaluated model cannot be identified safely", func() {
				So(decision.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When the retained forecast belongs to another symbol", func() {
			decision.Forecast.Symbol = "ETH/USD"

			Convey("Then cross-symbol provenance is rejected", func() {
				So(decision.Validate(), ShouldNotBeNil)
			})
		})
	})
}
